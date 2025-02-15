# Layotto (L8):To be the next layer of OSI layer 7

[![codecov](https://codecov.io/gh/mosn/layotto/branch/main/graph/badge.svg?token=10RxwSV6Sz)](https://codecov.io/gh/mosn/layotto)
[![Average time to resolve an issue](http://isitmaintained.com/badge/resolution/mosn/layotto.svg)](http://isitmaintained.com/project/mosn/layotto "Average time to resolve an issue")

<img src="https://raw.githubusercontent.com/mosn/layotto/main/docs/img/logo/grey2-1.svg" height="120px">

Layotto(/leɪˈɒtəʊ/) 是一款使用 Golang 开发的应用运行时, 旨在帮助开发人员快速构建云原生应用，帮助应用和基础设施解耦。它为应用提供了各种分布式能力，比如状态管理，配置管理，事件发布订阅等能力，以简化应用的开发。

Layotto 以开源的 [MOSN](https://github.com/mosn/mosn) 为底座，在提供分布式能力以外，提供了 Service Mesh 对于流量的管控能力。

## 诞生背景

Layotto希望可以把 [Multi-Runtime](https://www.infoq.com/articles/multi-runtime-microservice-architecture/) 跟Service Mesh两者的能力结合起来，无论你是使用MOSN还是Envoy或者其他产品作为Service Mesh的数据面，都可以在不增加新的sidecar的前提下，使用Layotto为这些数据面追加Runtime的能力。

例如，通过为MOSN添加Runtime能力，一个Layotto进程可以[既作为istio的数据面](zh/start/istio/start.md) 又提供各种Runtime API（例如Configuration API,Pub/Sub API等）

此外，随着探索实践，我们发现sidecar能做的事情远不止于此。 通过引入[WebAssembly](https://en.wikipedia.org/wiki/WebAssembly) ,我们正在尝试将Layotto做成 FaaS (Function as a service) 和 [reloadable sdk](https://github.com/mosn/layotto/issues/166) 的运行时容器 。

如果您对诞生背景感兴趣，可以看下[这篇演讲](https://mosn.io/layotto/#/zh/blog/mosn-subproject-layotto-opening-a-new-chapter-in-service-grid-application-runtime/index) 。

## 功能

- 服务通信 
- 服务治理，例如流量的劫持和观测，服务限流等
- [作为 istio 的数据面](zh/start/istio/start.md)  
- 配置管理
- 状态管理
- 事件发布订阅
- 健康检查、查询运行时元数据
- 基于WASM的多语言编程

## 工程架构

如下图架构图所示，Layotto 以开源 MOSN 作为底座，在提供了网络层管理能力的同时提供了分布式能力，业务可以通过轻量级的 SDK 直接与 Layotto 进行交互，而无需关注后端的具体的基础设施。

Layotto 提供了多种语言版本的 SDK，SDK 通过 gRPC 与 Layotto 进行交互，应用开发者只需要通过 Layotto 提供的配置文件[配置文件](https://github.com/mosn/layotto/blob/main/configs/runtime_config.json)
来指定自己基础设施类型，而不需要进行任何编码的更改，大大提高了程序的可移植性。

![系统架构图](https://raw.githubusercontent.com/mosn/layotto/main/docs/img/runtime-architecture.png)


## 快速开始

### Get started with Layotto

您可以尝试 demo [通过 Layotto 调用 apollo 配置中心](zh/start/configuration/start-apollo.md) 来体验Layotto

其他功能的demo见下.

### API

|  API            | status |                               quick start                             |                                components                                 | desc |
|  -------------  | :----: | :--------------------------------------------------------------------:|:-------------------------------------------------------------------------:|---- |
| State           | ✅     | [demo](https://mosn.io/layotto/#/zh/start/state/start)                | [list](https://mosn.io/layotto/#/zh/component_specs/state/common)         | 提供读写KV模型存储的数据的能力 |
| Pub/Sub         | ✅     | [demo](https://mosn.io/layotto/#/zh/start/pubsub/start)               | [list](https://mosn.io/layotto/#/zh/component_specs/pubsub/redis)         | 提供消息的发布/订阅能力|
| Service Invoke  | ✅     | [demo](https://mosn.io/layotto/#/zh/start/rpc/helloworld)             | [list](https://mosn.io/layotto/#/zh/start/rpc/helloworld)                 | 通过 MOSN 进行服务调用|
| Config          | ✅     | [demo](https://mosn.io/layotto/#/zh/start/configuration/start-apollo) | [list](https://mosn.io/layotto/#/zh/component_specs/configuration/apollo) | 提供配置增删改查及订阅的能力|
| Lock            | ✅     | [demo](https://mosn.io/layotto/#/zh/start/lock/start)                 | [list](https://mosn.io/layotto/#/zh/component_specs/lock/common)          | 提供 lock/unlock 分布式锁的实现|
| Sequencer       | ✅     | [demo](https://mosn.io/layotto/#/zh/start/sequencer/start)            | [list](https://mosn.io/layotto/#/zh/component_specs/sequencer/common)     | 提供获取分布式自增ID的能力 |
| File            | ✅     | [demo](https://mosn.io/layotto/#/zh/start/file/start)                 | [list](https://mosn.io/layotto/#/zh/component_specs/file/common)          | 提供访问文件的能力 |
| Binding         | ✅     | TODO                                                                  | TODO                                                                      | 提供透传数据的能力 |


### 可扩展性
|  feature       | status |                         quick start                       |                         desc                         |
|  ------------- | :----: | :--------------------------------------------------------:|------------------------------------------------------|
| API 插件         | ✅     | [demo](https://mosn.io/layotto/#/zh/start/api_plugin/helloworld)   |  为 Layotto 添加您自己的API |


### Actuator

|  feature       | status |                         quick start                       |               desc               |
|  ------------- | :----: | :--------------------------------------------------------:|----------------------------------|
| Health Check   | ✅     | [demo](https://mosn.io/layotto/#/zh/start/actuator/start) | 查询Layotto依赖的各种组件的健康状态  |
| Metadata Query | ✅     | [demo](https://mosn.io/layotto/#/zh/start/actuator/start) | 查询Layotto或应用对外暴露的元信息    |

### 流量控制

|  feature      | status |                              quick start                              |                desc               |
|  -----------  | :----: | :--------------------------------------------------------------------:|-----------------------------------|
| TCP Copy      | ✅     | [demo](https://mosn.io/layotto/#/zh/start/network_filter/tcpcopy)     | 把Layotto收到的TCP数据dump到本地文件 |
| Flow Control  | ✅     | [demo](https://mosn.io/layotto/#/zh/start/stream_filter/flow_control) | 限制访问Layotto对外提供的API        |

### 使用WASM进行多语言编程

|  feature       | status |                       quick start                      |                               desc                         |
|  ------------- | :----: | :-----------------------------------------------------:|------------------------------------------------------------|
| Go (TinyGo)    | ✅     | [demo](https://mosn.io/layotto/#/zh/start/wasm/start)  | 把用 TinyGo 开发的代码编译成 *.wasm文件跑在 Layotto 上         |
| Rust           | 待开发     |  待开发 | 把用 Rust 开发的代码编译成 *.wasm文件跑在 Layotto 上           |
| AssemblyScript | 待开发     |  待开发 | 把用 AssemblyScript 开发的代码编译成 *.wasm文件跑在 Layotto 上 |

### 通过WebAssembly (WASM)做扩展、写FaaS

|  feature       | status |                       quick start                      |                               desc                         |
|  ------------- | :----: | :-----------------------------------------------------:|------------------------------------------------------------|
| Go (TinyGo)    | ✅     | [demo](https://mosn.io/layotto/#/zh/start/faas/start)  | 把用 TinyGo 开发的代码编译成 *.wasm文件跑在 Layotto 上，并且使用k8s进行调度。         |
| Rust           | 待开发     | 待开发 | 把用 Rust 开发的代码编译成 *.wasm文件跑在 Layotto 上，并且使用k8s进行调度。           |
| AssemblyScript | 待开发     | 待开发 | 把用 AssemblyScript 开发的代码编译成 *.wasm文件跑在 Layotto 上，并且使用k8s进行调度。 |

### Service Mesh
| feature | status |                       quick start                      |            desc            |
| ------- | :----: | :-----------------------------------------------------:|----------------------------|
| istio   | ✅     | [demo](https://mosn.io/layotto/#/zh/start/istio/start) | 跟istio集成，作为它的数据面   |


## 设计文档

[Actuator设计文档](zh/design/actuator/actuator-design-doc.md)

[pubsub api以及与dapr component的兼容性](zh/design/pubsub/pubsub-api-and-compability-with-dapr-component.md)

[configuration-api-with-apollo(英文)](en/design/configuration/configuration-api-with-apollo.md)

[rpc设计文档](zh/design/rpc/rpc设计文档.md)

[分布式锁api设计文档](zh/design/lock/lock-api-design.md)

[FaaS 设计文档](zh/design/faas/faas-poc-design.md)

## Landscapes

<p align="center">
<img src="https://landscape.cncf.io/images/left-logo.svg" width="150"/>&nbsp;&nbsp;<img src="https://landscape.cncf.io/images/right-logo.svg" width="200"/>
<br/><br/>
Layotto enriches the <a href="https://landscape.cncf.io/serverless">CNCF CLOUD NATIVE Landscape.</a>
</p>

## 社区

| 平台  | 联系方式        |
|:----------|:------------|
| 💬 [钉钉](https://www.dingtalk.com/zh) (推荐) | 群号: 31912621 或者扫描下方二维码 <br> <img src="https://raw.githubusercontent.com/mosn/layotto/main/docs/img/ding-talk-group-1.png" height="200px">

[comment]: <> (| 💬 [微信]&#40;https://www.wechat.com/&#41;  | 扫描下方二维码添加好友，她会邀请您加入微信群 <br> <img src="../img/wechat-group.jpg" height="200px">)

## 如何贡献代码

[组件开发指南](zh/development/developing-component.md)

[Layotto贡献者指南](zh/development/CONTRIBUTING.md)

## FAQ

### 跟dapr有什么差异？

dapr是一款优秀的Runtime产品，但它本身缺失了Service Mesh的能力，而这部分能力对于实际在生产环境落地是至关重要的，因此我们希望把Runtime
跟Service Mesh两种能力结合在一起，满足更复杂的生产落地需求。
