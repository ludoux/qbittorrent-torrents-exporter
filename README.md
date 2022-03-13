# qbittorrent-torrents-exporter 0.1.0
本工具需要Web API 功能打开的**运行中的 qbittorrent 客户端**和其配套的 **`BT_backup` 文件夹**。

本工具用于从 BT_backup 中导出 torrent 文件并重命名，并基于 Web API 来支持对 tracker、标签、分类、保存路径等筛选导出。同时针对 qbittorrent 4.4.x 版本会将 tracker 信息从 .torrent 文件中移除一改变，工具支持自动从 .fastresume 文件中读取并还原进导出的 .torrent 文件。

请确保 `BT_backup` 文件夹（内含 .torrent 和 .fastresume 文件）与本工具处在同一目录下，导出的种子文件将会在新建的名为 `export` 的文件夹内。

This tool need a **running qbittorrent program** with its Web API supported and its **`BT_backup` folder**.

It can help you export torrent files from BT_backup and automatically rename them, with tracker/tag/category/path-filter supported. Besides, it can automatically append missing trackers into exported .torrent file as qbittorrent 4.4.x will remove tracker information from .torrent file.

Please make sure the `BT_backup` folder(.torrent and .fastresume files) is under the same folder with exporter, and the exported torrent files will in a new-create folder named `export`.

## 使用方法 | How to use

导出目录 | Exported Path: `export/<path>/<trackerhost>/<category>/`

导出文件名 | Exported Filename: `[<tags>][<state>]<name>.torrent`

### 纯参数运行 | Running with arguments only

| 参数名          | 解释                                                         | 示例                                     |
| --------------- | ------------------------------------------------------------ | ---------------------------------------- |
| `qh`            | *qBittorrent Host Url (Web API)                              | `-qh "http://127.0.0.1:6363"`            |
| `qu`            | 登录用户名 \| qBittorrent Username                           | `-qu "admin"`                            |
| `qp`            | 登录密码 \| qBittorrent Password                             | `-qp "password"`                         |
| `fc`            | 需要导出的分类(留空全导出) \| Categories to be exported(Stay blank to export all) | `-fc "cate1,cate2"` or `-fc ""`          |
| `fp`            | 需要导出的保存路径(留空全导出) \| SavePaths to be exported(Stay blank to export all) | `-fp "path1,path2"` or `-fp ""`          |
| `ft`            | 需要导出的标签(留空全导出) \| Tag to be exported(Stay blank to export all) | `-ft "tag1,tag2"` or `-ft ""`            |
| `fth`           | 需要导出的tracker(留空全导出) \| Tracker to be exported(Stay blank to export all) | `-fth "leech.com,club.net"` or `-fth ""` |
| `at`            | 将导出的任务打上此标签名(留空不打) \| Tag exported torrent task(Stay blank to not tag) | `-at "exported"` or `-at ""`             |
| `githubchannel` | Force to use github channel to check update instead of using gitee channel | `-githubchannel`                         |

```
❯ ./qbittorrent-torrents-exporter -h
Usage of ./main:
  -at string
        AppendTag (default "-")
  -fc string
        CategoryFilter (default "-")
  -fp string
        PathFilter (default "-")
  -ft string
        TagFilter (default "-")
  -fth string
        TrackerHostFilter (default "-")
  -githubchannel
        Force to use github channel to check update instead of gitee channel.
  -qh string
        qBittorrent host. ex: http://127.0.0.1:6363
  -qp string
        qBittorrent password.
  -qu string
        qBittorrent usrname.
```

PS: 如果不传入 `qh` 参数，则会使用引导交互来在终端输入登录信息等；如果不传入 `fc` `fp` `ft` `fth` `at` 中任一参数，则会使用交互引导来在终端输入筛选信息等。

PS: If  `qh` is unset, tool will ask users to give login information. If `fc` `fp` `ft` `fth` and `at` are all unset, tool will ask users to give filter information.

### 交互式运行 | Running directly

不传递任何参数或者只传递 `-githubchannel` 参数，软件则会以交互方式让用户提供相关信息。

Tool will ask users to provide information if all arguments are unset or only `-githubchannel` is set.

### 截图 | Screenshots

![](./README.assets/screenshot-with_arguments.png)

![](./README.assets/screenshot-tree.png)
