业务服务的基础框架

### framework
- go-micro
- gin

### features
- [X] 服务注册/发现
- [ ] memoryCache [go-freelru]
- [X] go-micro
- [X] api 服务选择策略

#### api 服务选择策略
- 尽量做到负载均衡
- 在所有服务稳定时可以做到选择具有一致性
- 当服务不可用时只影响选择过它的请求
