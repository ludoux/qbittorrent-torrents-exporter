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
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	bencode "github.com/jackpal/bencode-go"
	"github.com/nohajc/go-qbittorrent/qbt"
	"github.com/spf13/cast"
	"github.com/wxnacy/wgo/arrays"
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
)

var version string = "0.3.1"

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
	hostReg := regexp.MustCompile(`((http://)|(https://)|(tcp://)|(udp://)|(ws://)|(wss://))?(?P<main>[^/]+?)(:\d+)?/`)
	ipv4IpReg := regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)
	urls := strings.Split(trackers, "(split)")
	result := ""
	preHost := ""
	warningFlag := false
	for _, url := range urls {
		matchs := hostReg.FindStringSubmatch(url)
		groupNames := hostReg.SubexpNames()
		tmpHost := ""
		for i, v := range groupNames {
			if v == "main" {
				tmpHost = matchs[i]
			}
		}
		if tmpHost == "" {
			return "", errors.New("Can't get main host!")
		}
		if tmpHost[0] == '[' {
			//ipv6
			result = tmpHost
		} else if ipv4IpReg.MatchString(tmpHost) {
			//ipv4
			result = tmpHost
		} else {
			//domain
			sp := strings.Split(tmpHost, ".")
			gLTD := []string{"com", "net", "org", "co"}
			if len(sp) >= 3 {
				if arrays.StringsContains(gLTD, sp[len(sp)-2]) != -1 {
					// tracker.example.com.us will return as example.com.us
					result = sp[len(sp)-3] + "." + sp[len(sp)-2] + "." + sp[len(sp)-1]
				} else {
					result = sp[len(sp)-2] + "." + sp[len(sp)-1]
				}
			} else {
				result = sp[len(sp)-2] + "." + sp[len(sp)-1]
			}
		}
		if preHost == "" {
			preHost = result
		} else if preHost != result && !warningFlag {
			fmt.Println("warning: different trackerhost in the same torrent! Will use the last trackerhost for filter.")
			warningFlag = true
			preHost = result
		}
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

	qb.Login(qbt.LoginOptions{Username: username, Password: password})
	// not required when 'Bypass from localhost' setting is active.
	singlelimit := 100
	for i := 0; true; i++ {
		filters := map[string]string{
			"limit":  cast.ToString(singlelimit),
			"sort":   "added_on",
			"offset": cast.ToString(singlelimit * i),
		}
		//offset 即为忽略前面多少个，当超过之后，会返回offset=0返回的值
		torrents, _ := qb.Torrents(filters)

		if _, dupe := hash2Torrent[torrents[0].Hash]; dupe {
			break
		}

		fmt.Println("get torrents: " + cast.ToString(singlelimit*i+1) + "~" + cast.ToString(singlelimit*i+len(torrents)))
		for _, torrent := range torrents {
			/*if !strings.Contains(torrent.State, "UP") && torrent.State != "uploading" {
				fmt.Println(torrent.Name, "处于", torrent.State, "状态，跳过")
				continue
			}*/
			if torrent.Tracker == "" { //when have multy trackers
				trackers, _ := qb.TorrentTrackers(torrent.Hash)
				first := true
				for _, v := range trackers {
					if v.Tier >= 0 { //Tier < 0 is used as placeholder when tier does not exist for special entries (such as DHT).
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
		//0.010s
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
	//fmt.Println(data)
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
	//目录-分类-[tracker][tag1,tag2]name.torrent
	//目录-tracker-分类-[tag1,tag2]name.torrent
	//目录-tracker-[分类][tag1,tag2]name.torrent

	//style := 2
	//pathStyle := []string{"<SafePath>/<tracker>/<category>/"}
	//fmt.Println("目前有四种输出方式:")
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
		if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ") Not Found in BT_backup")
			errorCount++
			continue
		}
		_, err = os.Stat("BT_backup/" + hash + ".fastresume")
		if err != nil {
			fmt.Println("Error: ", torrent.Name, "(", torrent.Hash, ") .fastresume Not Found in BT_backup")
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
			qb440 = " [qB4.4.x trackers fixed]"
			tks := decodeFastResume("BT_backup/" + hash + ".fastresume")
			appendAnnounce(curPath+curFilename, tks)
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
}

func main() {
	flag.Parse()
	fmt.Println("github.com/ludoux/qbittorrent-torrents-exporter v" + version)
	checkUpdateUrl := "https://gitee.com/ludoux/check-update/raw/master/qbittorrent-torrents-exporter/version.txt"
	if githubChannel {
		checkUpdateUrl = "https://raw.githubusercontent.com/ludoux/qbittorrent-torrents-exporter/master/version.txt"
	}
	resp, err := http.Get(checkUpdateUrl)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if string(body) != version {
		fmt.Println("New version found!")
	}
	if qbUrl == "" {
		fmt.Print("qBittorrent host url(ex http://127.0.0.1:6363 ):")
		fmt.Scanln(&qbUrl)
		fmt.Print("qBittorrent username:")
		fmt.Scanln(&qbUsername)
		fmt.Print("qBittorrent password:")
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
