# httpie2curl

[English](README.md) · **简体中文**

把 [HTTPie](https://httpie.io/) 命令转换成等价的 `curl` 命令。

```console
$ httpie2curl http PUT pie.dev/put name=John age:=29 token==secret
curl -X PUT -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"name":"John","age":29}' 'http://pie.dev/put?token=secret'
```

## 为什么

HTTPie 的 request item 语法敲起来很舒服，但它不可移植：这条命令没法交给同事、没法粘进 runbook、也没法附在 bug report 里 —— 除非对方也装了 HTTPie。`curl` 才是通用语。这个工具在两者之间做翻译，于是这条请求在任何机器上都能跑，并且**在发出去之前就能被读懂**。

它是单个 Go 二进制，**没有任何第三方依赖**。

## 安装

```console
$ git clone https://github.com/zhiteng/httpie2curl.git
$ cd httpie2curl
$ go build -o httpie2curl .
```

`go.mod` 声明 Go 1.27。`CGO_ENABLED=0 go build` 产出静态二进制，可以直接拷到同 OS/架构的任意机器上。

## 用法

```
httpie2curl 0.1.0 — convert an HTTPie command into the equivalent curl command

usage:
  httpie2curl [--h2c-OPTION...] [http|https] <HTTPie arguments...>

examples:
  httpie2curl http POST pie.dev/post name=John age:=29
  httpie2curl https pie.dev/get 'X-Api-Key:secret' q==httpie
  httpie2curl --h2c-skip-unsupported http --session=dev pie.dev/get

Everything that is not a --h2c-* option is handed to the HTTPie parser, so an
HTTPie command line can be pasted almost verbatim. A leading http/https word is
consumed as HTTPie's program name and selects the default scheme.

tool options:
  --h2c-curl=PATH          curl binary to emit (default "curl")
  --h2c-with-defaults      reproduce HTTPie's transport defaults: add curl
                           --compressed, so gzip/deflate responses are decoded
                           the way HTTPie decodes them
  --h2c-skip-unsupported   warn and continue instead of failing on HTTPie options
                           that curl cannot express
  --h2c-multiline          print the command over multiple lines with backslashes
  --h2c-help               show this help
  --h2c-version            print the version
```

上面这段就是 `httpie2curl --h2c-help` 的原样输出 —— 程序的输出保持英文原样，不翻译。

## 示例

HTTPS + 一个 header、没有 body —— 不会添加任何 curl 本来就会做的事：

```console
$ httpie2curl https pie.dev/get 'X-Api-Key:secret'
curl -H 'X-Api-Key: secret' https://pie.dev/get
```

一个 form 字段加一个文件上传 —— 只要出现文件字段就会变成 `multipart/form-data`，`-F` 原样接收路径：

```console
$ httpie2curl http -f POST pie.dev/post 'name=John Smith' 'cv@~/files/data.xml'
curl -X POST -F 'name=John Smith' -F 'cv=@~/files/data.xml' http://pie.dev/post
```

`--h2c-multiline` 会折行长命令，并且保证每个 flag 和它的值在同一行：

```console
$ httpie2curl --h2c-multiline http pie.dev/post 'Authorization:Bearer TOKEN' \
    'Content-Type:application/json' 'items:=[1,2,3]' 'q==search terms'
curl -X POST -H 'Accept: application/json, */*;q=0.5' \
  -H 'Authorization: Bearer TOKEN' -H 'Content-Type: application/json' \
  -d '{"items":[1,2,3]}' 'http://pie.dev/post?q=search+terms'
```

curl 表达不了的 HTTPie 选项会被报出来，而不是被悄悄丢掉：

```console
$ httpie2curl http --session=dev pie.dev
httpie2curl: curl cannot express: --session=dev
httpie2curl: rerun with --h2c-skip-unsupported to convert anyway
$ echo $?
1
```

## 能转换什么

### 方法与 URL

- 开头的 `http` 或 `https` 词会被消费掉并决定默认 scheme，对应 HTTPie 的那两个可执行文件。`https example.org` → `https://example.org`，`http example.org` → `http://example.org`。
- `://` 形式会被补全：`http ://example.org` → `http://example.org`。
- 无 scheme 的 `host:port/path` 会被识别为 URL（`example.org:8080`），而 `Name:Value` 是 header。
- 方法取显式的位置词；没有时按 HTTPie 的规则推断：没有 body 用 `GET`，有数据用 `POST`。
- 当 `GET` 带 body 时会保留显式的 `-X GET`，否则 curl 一看到 `-d` 就会改用 `POST`。

### Request item

| HTTPie item | 含义 | curl |
| --- | --- | --- |
| `Name:Value` | 请求 header | `-H 'Name: Value'` |
| `Name:` | 取消一个默认 header | `-H Name:` |
| `Name;` | 值为空的 header | `-H 'Name;'` |
| `name==value` | query 参数（会被 URL 编码） | 折进 URL |
| `name=value` | data field | JSON 字符串值，`-f` 下是 form 字段 |
| `name:=json` | raw JSON 字段 | 校验后原样嵌入 |
| `name@file` | 文件上传（配合 `-f`/`--multipart`） | `-F 'name=@file'` |
| `name=@file`、`:@`、`==@`、`:=@` | 值从文件读取 | 内联，或 multipart 下用 `-F 'name=<file'` |

反斜杠转义会被遵守（`foo\==bar` 是 data field `foo=`），`--` 之后的 item 可以以减号开头。

### 序列化

默认 JSON，`-f` 下是 `application/x-www-form-urlencoded`，`--multipart` 或存在文件字段时是 `multipart/form-data`。JSON 的字符串值会被加引号，而 `:=` 的值在 `json.Valid` 校验后原样嵌入，所以数字、布尔、数组和嵌套对象都保持类型：`age:=29` 是数字 `29`，不是字符串 `"29"`。

### Transport 选项

| HTTPie | curl |
| --- | --- |
| `-v`、`--verbose` | `-v` |
| `--follow` | `-L` |
| `--max-redirects N` | `--max-redirs N` |
| `--timeout SEC` | `--max-time SEC` |
| `--verify no`、`--insecure` | `-k` |
| `--proxy URL` | `-x URL` |
| `--path-as-is` | `--path-as-is` |
| `--stream` | `-N` |
| `-a`、`--auth USER:PASS` | `-u USER:PASS` |
| `--bearer TOKEN` | `-H 'Authorization: Bearer TOKEN'` |
| `-o`、`--output FILE` | `-o FILE` |
| `-b`、`--body`、`--raw RAW` | `-d RAW` |
| `--default-scheme SCHEME` | 用于无 scheme 的 URL 的 scheme |

## 生成出来的命令实际发送什么

目标是一条等价的请求，所以 HTTPie 隐式添加的 header 会被复现：

- JSON 数据 → `Content-Type: application/json` **以及** `Accept: application/json, */*;q=0.5`。这个 `Accept` 不是可有可无的：curl 自己的默认值是 `*/*`，会改变服务端的内容协商结果。
- `-f` → `Content-Type: application/x-www-form-urlencoded; charset=utf-8`。
- multipart → 不发 `Content-Type`，因为 boundary 必须由 curl 自己生成。
- 没有数据时，HTTPie 发的是 `Accept: */*`，本来就和 curl 的默认一致，所以什么都不发。

Shell 引号用的是 POSIX 规则。只有下面这些字符保持裸写：`A-Za-z0-9` 以及 `_@%+=:,./-`。其余的一律单引号包裹 —— 包括 `~`、`?`、`&`、`*`，这样值里的 `~` 永远不会被 shell 展开，query string 也不可能把命令丢到后台：

```console
$ httpie2curl http pie.dev/search q==httpie
curl 'http://pie.dev/search?q=httpie'
```

## curl 无法表达的选项

没有 curl 对应物的 HTTPie 选项（`--session`、`--print`/`--pretty`、`--style`、`--offline`、`--cert`、`-A`/`--auth-type=digest`、`--check-status`、plugin……）会被收集并报出来。**默认这是错误**（退出码 `1`），`--h2c-skip-unsupported` 才把它降级成警告 —— 这样"只转换了一半"不可能蒙混过关。

对于那些 HTTPie 要求带值的选项，值会和 flag 一起被消费掉，绝不会被误当成 URL：

```console
$ httpie2curl http --style solarized pie.dev
httpie2curl: curl cannot express: --style=solarized
```

## 退出码

| 码 | 含义 |
| --- | --- |
| `0` | 转换成功（stderr 上可能带提示） |
| `1` | 转换失败：无法识别的参数、非法的 raw JSON、文件读不到、请求里没有 URL，或者存在 curl 无法表达的 HTTPie 选项且没给 `--h2c-skip-unsupported` |
| `2` | 用法错误：未知的 `--h2c-*` 选项，或者根本没有可转换的 HTTPie 参数 —— 裸跑 `httpie2curl`，或者只给了程序名如 `httpie2curl http` |

## 已知限制

- **批量转换还没实现。** 转换脚本或 shell history 里的那些 `http …` 行是计划中的功能。
- **反向（`curl` → HTTPie）还没实现。** curl 的 `-d`/`--data-binary`/`--data-urlencode`/`-G`/`-F`/`--json` 语义互相重叠、多对一，所以反向转换是有损的，需要显式报告"不确定项"，而不是假装无损。
- **基于文件的 item 是在转换时读盘的**，所以工具会碰文件系统、自己展开 `~`、并且会去掉尾部 CR/LF。而 `name@file` 上传是把路径交给 curl（`-F 'name=@file'`），**curl 不会展开 `~`**，所以 `cv@~/x` 会保持字面量。
- `:=` 在 form 和 multipart 下没有表示法：`-f`/`--multipart` 时会被记为提示并丢弃。
- 显式 `Content-Type` 叠加 multipart 没有被处理（boundary 必须归 curl 管），会被报出来。
- URL 与 header 的消歧是启发式的。裸词是 URL；URL 位置之前的全大写裸词会被读成方法（所以 `http AHOY pie.dev` 能用，但一个无 scheme 的全大写主机名不行）。
- 引号只做 POSIX；输出不是 PowerShell 安全的。
- HTTPie 的 `User-Agent: HTTPie/<version>` 永远不会被复现 —— curl 会发自己的。`--h2c-with-defaults` 通过加 `--compressed` 覆盖了 `Accept-Encoding`。
- 未知的 HTTPie 选项默认被当作不取值，除非它出现在已知的取值选项表里。

## 开发

```console
$ go test ./...     # 表驱动转换用例、引号处理与错误路径
$ gofmt -l .
$ go vet ./...
```

- `main.go` —— CLI：`--h2c-*` 解析、程序名词、输出、退出码。
- `internal/convert/item.go` —— request item 语法：分隔符、转义、URL 与 header 的消歧。
- `internal/convert/parse.go` —— HTTPie argv → `Request`。
- `internal/convert/render.go` —— `Request` → curl argv，含隐式的 `Content-Type`/`Accept` 以及 JSON/form/multipart body。
- `internal/convert/shell.go` —— POSIX 引号与多行输出。

语法对齐 HTTPie 3.2.4 的 CLI 文档。转换另外做了端到端验证：把生成的命令真的执行到本地回显服务器上，比对服务端收到的 method、path、header 和 body。

## 状态

初始版本（`0.1.0`）：上面列出的正向转换面已经完整。批量转换、反向转换和 LICENSE 还没做。
