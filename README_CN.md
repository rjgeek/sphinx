
<h1 align="center">Sphinx</h1>
<h4 align="center">Version 0.0.1</h4>

欢迎来到[Sphinx]的源码库！ 

[English](README.md) | 中文

Sphinx致力于创建一个高性能、分布式信任协作平台。让部署及调用去中心化应用变得更加简单。

一些新的功能还处于快速的开发过程中，master代码可能是不稳定的，稳定的版本可以在 [releases](https://github.com/fast-box/Sphinx/releases) 中下载。

非常欢迎及希望能有更多的开发者加入到Sphinx中来。

## 构建开发环境
成功编译Sphinx需要以下准备：

* Golang版本在1.12及以上
* 正确的Go语言开发环境
* Golang所支持的操作系统

## 获取Sphinx

### 从release获取
您可以从[release](https://github.com/rjgeek/Sphinx/releases) 处下载稳定版本的Sphinx节点程序.

### 从源码获取
克隆Sphinx仓库到 **$GOPATH/src/github.com/rjgeek** 目录

```shell
$ git clone https://github.com/rjgeek/Sphinx.git
```
或者
```shell
$ go get github.com/rjgeek/Sphinx
```

用make编译源码

```shell
$ cd Sphinx
$ make all
```

成功编译后会在`build/bin`目录下生成两个可以执行程序

* `shx`: 节点程序/以命令行方式提供的节点控制程序
* `promfile`: 用来创建创世文件的程序

## 运行Sphinx
`Sphinx`可以运行节点连接到测试链或者建立私有网络，请在[这里](https://github.com/rjgeek/Sphinx/wiki) 查看详细步骤。
## 示例
查看详细的示例，请点击[这里](https://github.com/rjgeek/Sphinx)
## 贡献代码

请您以签过名的commit发送pull request请求，我们期待您的加入！

另外，在您想为本项目贡献代码时请参考[contributing guidelines](CONTRIBUTING.md)。

您也可以[点击此处](https://github.com/rjgeek/Sphinx/issues/new) 提出您的问题。

## 许可证

Sphinx 遵守GNU Lesser General Public License, 版本3.0。 详细信息请查看项目根目录下的[LICENSE](LICENSE)文件。
