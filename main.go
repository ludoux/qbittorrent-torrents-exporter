/*
https://github.com/ludoux/qbittorrent-torrents-exporter

ludoux/qbittorrent-torrents-exporter is licensed under the
GNU General Public License v3.0

Lu Chang (chinaluchang@live.com)
*/
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	bencode "github.com/jackpal/bencode-go"
	"github.com/nohajc/go-qbittorrent/qbt"
	"github.com/spf13/cast"
	"github.com/weppos/publicsuffix-go/publicsuffix"
)

var qb *qbt.Client

var (
	githubChannel             bool
	qbUrl                     string
	qbUsername                string
	qbPassword                string
	filterPath                string
	filterTrackerHost         string
	filterCategory            string
	filterTag                 string
	appendTag                 string
	disableTrackerHostAnalize bool = false
	debug                     bool = false
)

var version string = "0.3.5"

type hashsPair struct {
	hash2Torrent      map[string]qbt.BasicTorrent
	savePath2Hashs    map[string][]string
	trackerHost2Hashs map[string][]string
	category2Hashs    map[string][]string
	tag2Hashs         map[string][]string
}

type filterOptions struct {
	pathFilter        []string
	trackerHostFilter []string
	categoryFilter    []string
	tagFilter         []string
}

func getTrackerHost(trackers string) (string, error) {
	if disableTrackerHostAnalize {
		return "_tracker_", nil
	}
	if debug {
		fmt.Println("==[Debug]==\nRaw Trackers: " + trackers)
	}
	if len(trackers) == 0 {
		return "_no_tracker_", nil
	}
	urls := strings.Split(trackers, "(split)")
	result := ""
	preHost := ""
	warningFlag := false
	for _, ur := range urls {
		url, err := url.Parse(ur)
		if err != nil {
			return "_tracker_", errors.New("can't get main host! The raw trackers is " + trackers)
		}
		tmpHost := url.Hostname()

		fmt.Println(url.Hostname())
		if tmpHost == "" {
			return "_tracker_", errors.New("can't get main host! The raw trackers is " + trackers)
		}
		if tmpHost[0] == '[' {
			//ipv6
			result = tmpHost
		} else if net.ParseIP(tmpHost) != nil {
			//可以转换为IP
			result = tmpHost
		} else {
			//domain
			result, err = publicsuffix.Domain(tmpHost)
			if err != nil {
				return "_tracker_", errors.New("can't get main host! The raw trackers is " + trackers)
			}
		}
		if preHost == "" {
			preHost = result
		} else if preHost != result && !warningFlag {
			fmt.Println("Warning: different trackerhost in the same torrent! Will use the last trackerhost for filter.")
			warningFlag = true
			preHost = result
		}
	}
	if debug {
		fmt.Println("Result: " + result + "\n==[Off]==")
	}
	return result, nil
}
func genMap(url string, username string, password string) (hashsPair, error) {
	rt := hashsPair{}
	//========分类存储
	hash2Torrent := map[string]qbt.BasicTorrent{}
	savePath2Hashs := map[string][]string{}
	trackerHost2Hashs := map[string][]string{} // host.com
	category2Hashs := map[string][]string{}
	tag2Hashs := map[string][]string{}

	qb = qbt.NewClient(url)

	err := qb.Login(qbt.LoginOptions{Username: username, Password: password})
	if err != nil {
		log.Fatal("Error: Login failed. Please check your url, username and password: ", err)
	}
	singlelimit := 100
	for i := 0; true; i++ {
		filters := map[string]string{
			"limit":  cast.ToString(singlelimit),
			"sort":   "added_on",
			"offset": cast.ToString(singlelimit * i),
		}
		//offset 即为忽略前面多少个，当超过之后，会返回offset=0返回的值
		torrents, _ := qb.Torrents(filters)
		if len(torrents) == 0 {
			break
		}
		if _, dupe := hash2Torrent[torrents[0].Hash]; dupe {
			break
		}

		fmt.Println("get torrents: " + cast.ToString(singlelimit*i+1) + "~" + cast.ToString(singlelimit*i+len(torrents)))
		for _, torrent := range torrents {
			/*if !strings.Contains(torrent.State, "UP") && torrent.State != "uploading" {
				fmt.Println(torrent.Name, "处于", torrent.State, "状态，跳过")
				continue
			}*/
			torrent.Tracker = ""
			// Force to use "api/v2/torrents/trackers" to get trackers
			if true {
				trackers, _ := qb.TorrentTrackers(torrent.Hash)
				first := true
				for _, v := range trackers {
					if v.Tier >= 0 && v.Status != 0 { //Tier < 0 is used as placeholder when tier does not exist for special entries (such as DHT).
						if first {
							torrent.Tracker = v.URL
							first = false
						} else {
							torrent.Tracker = torrent.Tracker + "(split)" + v.URL
						}
					}
				}
			}
			hash2Torrent[torrent.Hash] = torrent

			savePath2Hashs[torrent.SavePath] = append(savePath2Hashs[torrent.SavePath], torrent.Hash)

			host, err := getTrackerHost(torrent.Tracker)
			if err != nil {
				return rt, err
			}
			trackerHost2Hashs[host] = append(trackerHost2Hashs[host], torrent.Hash)

			category2Hashs[torrent.Category] = append(category2Hashs[torrent.Category], torrent.Hash)

			tags := strings.Split(torrent.Tags, ",")
			for _, tag := range tags {
				tag2Hashs[tag] = append(tag2Hashs[tag], torrent.Hash)
			}
		}
		if len(torrents) < singlelimit {
			break
		}
		//10=0.010s
		time.Sleep(time.Duration(10) * time.Millisecond)
	}
	rt.hash2Torrent = hash2Torrent
	rt.savePath2Hashs = savePath2Hashs
	rt.trackerHost2Hashs = trackerHost2Hashs
	rt.category2Hashs = category2Hashs
	rt.tag2Hashs = tag2Hashs
	return rt, nil
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}
func copyTorrent(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	oriDst := dst
	for i := 1; true; i++ {
		_, err = os.Stat(dst)
		if err == nil {
			//same name file exist
			dst = strings.Replace(oriDst, ".torrent", "_"+cast.ToString(i)+".torrent", 1)
		} else {
			break
		}
	}

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
func toSafeFolderName(folderName string) string {
	folderName = strings.ReplaceAll(folderName, `//`, "#")
	folderName = strings.ReplaceAll(folderName, `\\`, "#")
	folderName = strings.ReplaceAll(folderName, `/`, "#")
	folderName = strings.ReplaceAll(folderName, `\`, "#")
	folderName = strings.ReplaceAll(folderName, `:`, "#")
	folderName = strings.ReplaceAll(folderName, `*`, "#")
	folderName = strings.ReplaceAll(folderName, `?`, "#")
	folderName = strings.ReplaceAll(folderName, `"`, "#")
	folderName = strings.ReplaceAll(folderName, `<`, "#")
	folderName = strings.ReplaceAll(folderName, `>`, "#")
	folderName = strings.ReplaceAll(folderName, `|`, "#")
	return folderName
}

func checkTorrentHasTracker(path string) bool {
	op, _ := os.Open(path)
	defer op.Close()
	data, err := bencode.Decode(op)
	if err != nil {
		panic(err)
	}
	rt := cast.ToStringMap(data)
	if rt["announce"] == nil {
		return rt["announce-list"] != nil
	} else {
		return true
	}
}

func decodeFastResume(path string) []string {

	op, _ := os.Open(path)
	defer op.Close()
	data, err := bencode.Decode(op)
	if err != nil {
		panic(err)
	}
	rt := cast.ToStringMap(data)

	t := rt["trackers"]
	slice := []string{}
	toTrackerSlice(t, &slice)
	return slice
}

func toTrackerSlice(t interface{}, rt *[]string) {
	vs, ok := t.([]interface{})
	if !ok {
		panic("fail")
	}
	for _, v := range vs {
		if reflect.TypeOf(v).Kind() == reflect.String {
			*rt = append(*(rt), cast.ToString(v))
		} else {
			toTrackerSlice(v, rt)
		}
	}
}

func appendAnnounce(path string, trackers []string) {
	op, _ := os.Open(path)
	data, err := bencode.Decode(op)
	op.Close()
	if err != nil {
		panic(err)
	}
	moded := cast.ToStringMap(data)
	moded["announce"] = trackers[0]
	if len(trackers) > 1 {
		aHead := [][]string{}
		aList := []string{}
		for i := 0; i < len(trackers); i++ {
			aList = append(aList, trackers[i])
		}
		aHead = append(aHead, aList)
		moded["announce-list"] = aHead
	}

	var buf bytes.Buffer
	err = bencode.Marshal(&buf, moded)
	if err != nil {
		panic(err)
	}
	e := os.WriteFile(path, buf.Bytes(), 0666)
	if e != nil {
		panic(e)
	}
}
func exportTorrentFiles(hashs *hashsPair, filter *filterOptions, appendTagName string) error {
	errorCount := 0
	doneHashs := []string{}
	curPathStyle := "export/<path>/<trackerhost>/<category>/"
	curFilenameStyle := "[<tags>][<state>]<name>.torrent"
	for hash, torrent := range hashs.hash2Torrent {
		host, _ := getTrackerHost(torrent.Tracker)
		if (len(filter.pathFilter) == 0 || contains(filter.pathFilter, torrent.SavePath)) && (len(filter.trackerHostFilter) == 0 || contains(filter.trackerHostFilter, host)) && (len(filter.categoryFilter) == 0 || contains(filter.categoryFilter, torrent.Category)) {
			tags := strings.Split(torrent.Tags, ",")
			hit := false
			for _, tag := range tags {
				if len(filter.tagFilter) == 0 || contains(filter.tagFilter, tag) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		} else {
			continue
		}

		//===pass check

		curPath := strings.ReplaceAll(curPathStyle, "<path>", toSafeFolderName(torrent.SavePath))
		h, err := getTrackerHost(torrent.Tracker)
		if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ") Failed to getTrackerHost. err:"+err.Error())
			errorCount++
			continue
		}
		if h == "_notracker_" {
			fmt.Println("Warning: ", torrent.Name, "(", torrent.Hash, ") 's tracker is empty.")
			errorCount++
			continue
		}
		curPath = strings.ReplaceAll(curPath, "<trackerhost>", toSafeFolderName(h))
		curPath = strings.ReplaceAll(curPath, "<category>", toSafeFolderName(torrent.Category))
		curPath = strings.ReplaceAll(curPath, "//", "/")
		os.MkdirAll(curPath, os.ModeDir|os.ModePerm)
		os.Create(curPath + "realpath.txt")
		err = os.WriteFile(curPath+"realpath.txt", []byte(torrent.SavePath), 0666)
		if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ") Failed to create realpath.txt. err:"+err.Error())
			errorCount++
			continue
		}
		curFilename := curFilenameStyle
		curFilename = strings.ReplaceAll(curFilename, "<tags>", toSafeFolderName(torrent.Tags))
		curFilename = strings.ReplaceAll(curFilename, "<state>", toSafeFolderName(torrent.State))
		curFilename = strings.ReplaceAll(curFilename, "<name>", toSafeFolderName(torrent.Name))
		_, err = os.Stat("BT_backup/" + hash + ".torrent")
		if err != nil && os.IsNotExist(err) {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ".torrent) Not Found in BT_backup")
			errorCount++
			continue
		} else if err != nil && os.IsPermission(err) {
			fmt.Println("Error: No permission to access BT_backup/", torrent.Hash, ".torrent for ", torrent.Name)
			errorCount++
			continue
		} else if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ".", "torrent) ", err.Error())
			errorCount++
			continue
		}

		_, err = os.Stat("BT_backup/" + hash + ".fastresume")
		if err != nil && os.IsNotExist(err) {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ".fastresume) Not Found in BT_backup")
			errorCount++
			continue
		} else if err != nil && os.IsPermission(err) {
			fmt.Println("Error: No permission to access BT_backup/", torrent.Hash, ".fastresume for ", torrent.Name)
			errorCount++
			continue
		} else if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ".", "fastresume) ", err.Error())
			errorCount++
			continue
		}

		_, err = copyTorrent("BT_backup/"+hash+".torrent", curPath+curFilename)
		if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ") Copy .torrent failed. Please check your file permission. err:"+err.Error())
			errorCount++
			continue
		}
		//====qb4.4.x
		qb440 := ""
		if !checkTorrentHasTracker("BT_backup/" + hash + ".torrent") {

			tks := decodeFastResume("BT_backup/" + hash + ".fastresume")
			if len(tks) == 0 {
				qb440 = " [no tracker found in" + hash + ".torrent/fastresume]"
			} else {
				qb440 = " [qB4.4.x trackers fixed]"
				appendAnnounce(curPath+curFilename, tks)
			}

		}

		fmt.Println("Done:", curPath, curFilename, qb440)
		doneHashs = append(doneHashs, hash)
	}

	if appendTagName != "" {
		for _, hash := range doneHashs {
			fTags := []string{}
			fTags = append(fTags, appendTagName)
			oriTags := strings.Split(hashs.hash2Torrent[hash].Tags, ",")
			fTags = append(fTags, oriTags...)
			ok, _ := qb.AddTorrentTags([]string{hash}, fTags)
			if !ok {
				fmt.Println("Error: ", hashs.hash2Torrent[hash].Name, "打tag失败!")
			}
		}

	}
	fmt.Println("Done.")
	if errorCount > 0 {
		fmt.Println("ErrorCount: " + cast.ToString(errorCount) + ". Please check the log above.")
	}
	return nil
}

func genSummary(hashs *hashsPair) string {
	rt := "==汇总/Summary==\n种子总数/Torrents Count: " + cast.ToString(len(hashs.hash2Torrent)) + "\nTrackers(" + cast.ToString(len(hashs.trackerHost2Hashs)) + "): \n."
	i := 0
	for tracker, subHashs := range hashs.trackerHost2Hashs {

		if i < len(hashs.trackerHost2Hashs)-1 {
			rt = rt + "\n├─" + cast.ToString(tracker) + "(" + cast.ToString(len(subHashs)) + ")"
		} else {
			rt = rt + "\n└─" + cast.ToString(tracker) + "(" + cast.ToString(len(subHashs)) + ")"
		}
		i = i + 1
	}
	rt = rt + "\n\n保存目录/Save Paths(" + cast.ToString(len(hashs.savePath2Hashs)) + "):\n."
	i = 0
	for savePath, subHashs := range hashs.savePath2Hashs {
		if i < len(hashs.savePath2Hashs)-1 {
			rt = rt + "\n├─" + cast.ToString(savePath) + "(" + cast.ToString(len(subHashs)) + ")"
		} else {
			rt = rt + "\n└─" + cast.ToString(savePath) + "(" + cast.ToString(len(subHashs)) + ")"
		}
		i = i + 1
	}
	rt = rt + "\n\n分类/Categories(" + cast.ToString(len(hashs.category2Hashs)) + "):\n."
	i = 0
	for category, subHashs := range hashs.category2Hashs {
		if i < len(hashs.category2Hashs)-1 {
			rt = rt + "\n├─" + cast.ToString(category) + "(" + cast.ToString(len(subHashs)) + ")"
		} else {
			rt = rt + "\n└─" + cast.ToString(category) + "(" + cast.ToString(len(subHashs)) + ")"
		}
		i = i + 1
	}

	rt = rt + "\n\n标签/Tags(" + cast.ToString(len(hashs.tag2Hashs)) + "):\n."
	i = 0
	for tag, subHashs := range hashs.tag2Hashs {
		if i < len(hashs.savePath2Hashs)-1 {
			rt = rt + "\n├─" + cast.ToString(tag) + "(" + cast.ToString(len(subHashs)) + ")"
		} else {
			rt = rt + "\n└─" + cast.ToString(tag) + "(" + cast.ToString(len(subHashs)) + ")"
		}
		i = i + 1
	}
	return rt
}
func setFilter(hashs *hashsPair, filterOp *filterOptions, appendTag *string) {
	var ordered []string //因为map为无序
	//count := 0
	var curMap *map[string][]string
	for i := 0; i < 4; i++ {
		ordered = make([]string, 0)
		switch i {
		case 0:
			fmt.Println("pathFilter:")
			curMap = &hashs.savePath2Hashs
		case 1:
			fmt.Println("trackerHostFilter:")
			curMap = &hashs.trackerHost2Hashs
		case 2:
			fmt.Println("categoryFilter:")
			curMap = &hashs.category2Hashs
		case 3:
			fmt.Println("tagFilter:")
			curMap = &hashs.tag2Hashs
		}
		for it := range *curMap {
			ordered = append(ordered, it)
		}
		sort.Strings(ordered)
		for i, it := range ordered {
			fmt.Printf("[%v]%v(%v)\n", i, it, len((*curMap)[it]))

		}
		fmt.Println("输入筛选项序号，以英文逗号分割。留空表示选择全部/Enter the filter item number, separated by commas. Leave blank to select all.")
		input := ""
		fmt.Scanln(&input)
		if input == "-1" || input == "" {
		} else {
			split := strings.Split(input, ",")
			for _, v := range split {
				num, _ := strconv.Atoi(v)

				switch i {
				case 0:
					filterOp.pathFilter = append(filterOp.pathFilter, ordered[num])
				case 1:
					filterOp.trackerHostFilter = append(filterOp.trackerHostFilter, ordered[num])
				case 2:
					filterOp.categoryFilter = append(filterOp.categoryFilter, ordered[num])
				case 3:
					filterOp.tagFilter = append(filterOp.tagFilter, ordered[num])

				}
			}
		}
	}
	fmt.Println("给导出的任务打tag?留空表示无需/To append tag for the exported torrents? Leave blank to ignore.")
	input := ""
	fmt.Scanln(&input)
	*appendTag = input
}
func init() {
	flag.BoolVar(&githubChannel, "githubchannel", false, "Force to use github channel to check update instead of gitee channel.")
	flag.StringVar(&qbUrl, "qh", "", "qBittorrent host. ex: http://127.0.0.1:6363")
	flag.StringVar(&qbUsername, "qu", "", "qBittorrent usrname.")
	flag.StringVar(&qbPassword, "qp", "", "qBittorrent password.")
	flag.StringVar(&filterPath, "fp", "-", "PathFilter")
	flag.StringVar(&filterTrackerHost, "fth", "-", "TrackerHostFilter")
	flag.StringVar(&filterCategory, "fc", "-", "CategoryFilter")
	flag.StringVar(&filterTag, "ft", "-", "TagFilter")
	flag.StringVar(&appendTag, "at", "-", "AppendTag")
	flag.BoolVar(&disableTrackerHostAnalize, "disableAnalize", false, "DisableTrackerHostAnalize")
	flag.BoolVar(&debug, "debug", false, "Debug")
}

func main() {
	_, err := os.Stat("BT_backup")
	if err != nil && os.IsNotExist(err) {
		log.Fatal("Error: the BT_backup folder does not exist.")
	} else if err != nil && os.IsPermission(err) {
		log.Fatal("Error: exporter has no permission to access BT_backup.")
	} else if err != nil {
		log.Fatal("Error:", err.Error())
	} else {
		dirEntry, err := os.ReadDir("BT_backup")
		if err != nil {
			log.Fatal("Error: Failed to read BT_backup path:", err)
		}
		if len(dirEntry) == 0 {
			log.Fatal("Error: There is no file in BT_backup folder.")
		}
		info, err := dirEntry[0].Info()
		if err != nil {
			log.Fatal("Error: Failed to read 1st file info:", err)
		}
		fmt.Println("( BT_backup check PASS. It has", len(dirEntry), "files, the 1st file is", dirEntry[0].Name(), "with the size of", info.Size(), ")")
	}
	flag.Parse()
	fmt.Println("github.com/ludoux/qbittorrent-torrents-exporter v" + version)
	if qbUrl == "" {
		fmt.Print("qBittorrent host url (ex http://127.0.0.1:6363 ):")
		fmt.Scanln(&qbUrl)
		if !strings.HasPrefix(qbUrl, "http") {
			qbUrl = "http://" + qbUrl
		}
		fmt.Print("qBittorrent username (press Enter directly if autologon):")
		fmt.Scanln(&qbUsername)
		fmt.Print("qBittorrent password (press Enter directly if autologon):")
		fmt.Scanln(&qbPassword)
	}
	hashs, err := genMap(qbUrl, qbUsername, qbPassword)
	if err != nil {
		panic(err)
	}
	fmt.Println(genSummary(&hashs))
	filterOptions := filterOptions{}
	if filterPath != "-" || filterTrackerHost != "-" || filterCategory != "-" || filterTag != "-" || appendTag != "-" {
		if filterPath != "-" {
			split := strings.Split(filterPath, ",")
			filterOptions.pathFilter = append(filterOptions.pathFilter, split...)
		}
		if filterTrackerHost != "-" {
			split := strings.Split(filterTrackerHost, ",")
			filterOptions.trackerHostFilter = append(filterOptions.trackerHostFilter, split...)
		}
		if filterCategory != "-" {
			split := strings.Split(filterCategory, ",")
			filterOptions.categoryFilter = append(filterOptions.categoryFilter, split...)
		}
		if filterTag != "-" {
			split := strings.Split(filterTag, ",")
			filterOptions.tagFilter = append(filterOptions.tagFilter, split...)
		}
		if appendTag == "-" {
			appendTag = ""
		}
	} else {
		fmt.Println("\n===SetFilter===\n")
		setFilter(&hashs, &filterOptions, &appendTag)
	}
	fmt.Println("即将导出...")
	exportTorrentFiles(&hashs, &filterOptions, appendTag)
}
