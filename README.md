
<center><h1>Snow</h1></center>

[![arXiv](https://img.shields.io/badge/arXiv%20paper-2504.02676-b31b1b.svg)](https://arxiv.org/abs/2504.02676)&nbsp;

Snow是一个分布式广播项目，使用自研协议进行类似树状广播。Snow自带了一套服务发现机制，使用自身的广播机制进行自举，可用于节点之间的发现。它或许可以被用在：数据同步，文件分发，消息广播，发布订阅系统，视频会议。

它是一个基于TCP进行消息广播的中间件，它是完全去中心化的，集群可以通过我们自研的协议进行服务发现。Snow的Regular消息会产生一颗平衡二叉树，来使用树状结构进行广播，在我们协议的加持下，节点的正常加入和离开都不会导致稳定部分节点消息的丢失。

Coloring消息是基于Regular消息的变形，在略微增加消息流量的情况下，允许集群中一个节点因为异常崩溃而离开（集群中优雅下线是常态，由硬件和系统级别故障导致的意料之外的掉线并不常发生）而不丢失任何消息，Coloring消息还能容忍部分的掉队者节点导致的集群速度的下降。

使用方法如下，项目正在验证中，请勿用于生产环境。

~~~go
config, err := broadcast.NewConfig(configPath, f)
if err != nil {
    panic(err)
    return
}

server, err := broadcast.NewServer(config, action)
if err != nil {
    log.Println(err)
    return
}

//发送消息
server.RegularMessage(msg, common.UserMsg)
~~~

这是对论文[Snow: Self-organizing Broadcast Protocol for Cloud](https://arxiv.org/abs/2504.02676)的实现。

服务发现基于[SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)和[Lifeguard](https://arxiv.org/abs/1707.007)
