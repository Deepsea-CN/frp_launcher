# frp_launcher

frp启动器，支持载入明文或密文配置文件一键启动

支持单文件版，可分发一键启动连接

## #加载器版本

### 功能点：

明文和加密文件自动处理。

支持启动和停止 frpc 服务。

自动管理临时解密配置文件。

### 使用说明：

1.启动程序，选择1，输入配置文件.toml .enc路径加载配置文件

2.选择2启动frp

## #配置生成工具

支持为你的配置文件进行加密，也可以将已有的文件解密为明文

生成唯一匹配的启动器，只有对应的生成工具生成的启动器才可以使用工具加密的配置文件
