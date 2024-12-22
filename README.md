# frp_launcher

frp启动器，GUI管理你的frpc客户端

使用frp的多种协议进行内网穿透时需要客户端支持，本程序旨在方便优雅的启动frp进行内网穿透

未来功能：\
支持配置加密 \
一键分发客户端配置 \
~frp被控端支持~ √\
支持更多协议 \
~支持导入和导出配置文件~ √ 

## #GUI版本

可以增删改查多个配置文件，管理frp启停

### 功能点：

表单式添加配置

支持visitor和proxies多个配置

图形化查看和选择配置文件

多种方式导入和导出配置

支持图形化启动和停止 frpc 服务

实时查看日志

主题模式切换

### 使用说明：

添加配置：按需填入服务器，visitor，proxies信息（为保证安全，强制要求使用鉴权方式token）

导入配置：通过选择已有配置文件或base64导入

导出配置：可导出配置文件或base64字符串

修改配置：实时查看配置文件，可进行修改

删除配置：删除选中的配置文件

启动frp：选择配置文件后点击一键启动frp

停止frp：一键停止frp

切换主题：切换白天模式或黑暗模式

配置列表：实时查看和选择配置文件

实时日志：实时打印日志


## #配置生成工具（未来功能）

支持为你的配置文件进行加密，也可以将已有的文件解密为明文

生成唯一匹配的启动器，只有对应的生成工具生成的启动器才可以使用工具加密的配置文件
