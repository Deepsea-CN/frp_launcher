package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	srcDir      = "./src"
	configFiles []string
	selectedID  = -1    // 当前选中的配置文件索引
	isDarkMode  = false // 标记当前是否为黑夜模式
	frpProcess  *exec.Cmd
)

func main() {
	myApp := app.New()
	iconPath := filepath.Join("assets", "icon.ico") // 这里使用 .ico 文件路径
	icon, err := loadIconFromFile(iconPath)
	if err != nil {
		fmt.Println("加载图标失败:", err)
		return
	}
	window := myApp.NewWindow("FRP 控制器")
	window.SetIcon(icon)
	window.Resize(fyne.NewSize(800, 600))
	// 初始化目录
	err = initDirs()
	if err != nil {
		dialog.ShowError(err, window)
		return
	}

	// 配置列表
	configList := widget.NewList(
		func() int { return len(configFiles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(configFiles[i])
		},
	)

	// 监听选中项
	configList.OnSelected = func(id widget.ListItemID) {
		selectedID = id
	}

	// 刷新配置文件列表
	refreshConfigFiles := func() {
		files, err := os.ReadDir(srcDir)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取目录失败: %v", err), window)
			return
		}
		configFiles = nil
		for _, file := range files {
			if filepath.Ext(file.Name()) == ".toml" {
				configFiles = append(configFiles, file.Name())
			}
		}
		configList.Refresh()
	}
	refreshConfigFiles()

	// 日志区域
	logs := widget.NewMultiLineEntry()
	logs.Enable() // 使日志框不可编辑
	logs.Wrapping = fyne.TextWrapWord
	dir := "./logs"                                   // 日志文件夹
	logFilePath := filepath.Join(dir, "frp_logs.txt") // 保存日志的文件名
	overwriteLogs := true                             // 设置为 true 表示每次启动清空日志
	// 保留最新 n 行日志
	maxLogLines := 10
	// 更新日志显示函数
	updateLogDisplay := func(entry *widget.Entry, newLine string) {
		currentText := entry.Text
		lines := strings.Split(currentText, "\n")
		lines = append(lines, newLine)
		if len(lines) > maxLogLines {
			lines = lines[len(lines)-maxLogLines:]
		}
		// 更新日志框
		entry.SetText(strings.Join(lines, "\n"))
	}

	// 添加配置
	addConfigButton := widget.NewButton("新建配置", func() {
		serverAddr := widget.NewEntry()
		serverAddr.SetPlaceHolder("服务器地址")
		serverPort := widget.NewEntry()
		serverPort.SetPlaceHolder("服务器端口")
		authToken := widget.NewEntry()
		authToken.SetPlaceHolder("鉴权 Token")

		validateIP := func(ip string) bool {
			return net.ParseIP(ip) != nil
		}

		validatePort := func(port string) bool {
			p, err := strconv.Atoi(port)
			return err == nil && p >= 0 && p <= 65535
		}

		errorLabel := widget.NewLabel("")
		errorLabel.Hide()

		markInvalid := func(entry *widget.Entry, valid bool, errorMsg string) {
			if valid {
				entry.Validator = nil
				errorLabel.Hide()
			} else {
				entry.Validator = func(s string) error {
					return errors.New(errorMsg)
				}
				errorLabel.SetText(errorMsg)
				errorLabel.Show()
			}
		}

		visitors := []fyne.CanvasObject{}
		visitorList := container.NewVBox()
		addVisitorButton := widget.NewButton("添加 Visitor", func() {
			visitorName := widget.NewEntry()
			visitorName.SetPlaceHolder("Visitor 名称")
			visitorType := widget.NewSelect([]string{"xtcp", "stcp"}, func(selected string) {
				// 可以在此添加选择后的逻辑
			})
			visitorType.PlaceHolder = "类型"

			serverName := widget.NewEntry()
			serverName.SetPlaceHolder("服务器名称")
			secretKey := widget.NewEntry()
			secretKey.SetPlaceHolder("密钥")
			bindAddr := widget.NewEntry()
			bindAddr.SetPlaceHolder("绑定地址")
			bindPort := widget.NewEntry()
			bindPort.SetPlaceHolder("绑定端口")
			keepTunnelOpen := widget.NewCheck("保持隧道打开", nil)

			visitorForm := container.NewVBox(
				widget.NewLabel("Visitor 配置项"),
				visitorName,
				visitorType,
				serverName,
				secretKey,
				bindAddr,
				bindPort,
				keepTunnelOpen,
			)
			removeButton := widget.NewButton("移除", func() {
				visitorList.Remove(visitorForm)
				for i, v := range visitors {
					if v == visitorForm {
						visitors = append(visitors[:i], visitors[i+1:]...)
						break
					}
				}
			})
			visitorForm.Add(removeButton)
			visitors = append(visitors, visitorForm)
			visitorList.Add(visitorForm)
		})

		proxies := []fyne.CanvasObject{}
		proxyList := container.NewVBox()
		addProxyButton := widget.NewButton("添加 Proxy", func() {
			proxyName := widget.NewEntry()
			proxyName.SetPlaceHolder("Proxy 名称")
			proxyType := widget.NewSelect([]string{"tcp", "udp", "xtcp", "stcp"}, func(selected string) {
				// 可以在此添加选择后的逻辑
			})
			proxyType.PlaceHolder = "类型"

			localAddr := widget.NewEntry()
			localAddr.SetPlaceHolder("本地地址")
			localAddr.OnChanged = func(text string) {
				markInvalid(localAddr, validateIP(text), "无效的本地地址")
			}

			localPort := widget.NewEntry()
			localPort.SetPlaceHolder("本地端口")
			localPort.OnChanged = func(text string) {
				markInvalid(localPort, validatePort(text), "端口范围应为 0-65535")
			}

			remotePort := widget.NewEntry()
			remotePort.SetPlaceHolder("远程端口")
			remotePort.OnChanged = func(text string) {
				markInvalid(remotePort, validatePort(text), "端口范围应为 0-65535")
			}

			secretKey := widget.NewEntry()
			secretKey.SetPlaceHolder("密钥")

			proxyType.OnChanged = func(selected string) {
				if selected == "xtcp" {
					remotePort.Hide()
					secretKey.Show()
				} else {
					remotePort.Show()
					secretKey.Hide()
				}
			}

			proxyForm := container.NewVBox(
				widget.NewLabel("Proxy 配置项"),
				proxyName,
				proxyType,
				localAddr,
				localPort,
				remotePort,
				secretKey,
			)
			removeButton := widget.NewButton("移除", func() {
				proxyList.Remove(proxyForm)
				for i, p := range proxies {
					if p == proxyForm {
						proxies = append(proxies[:i], proxies[i+1:]...)
						break
					}
				}
			})
			proxyForm.Add(removeButton)
			proxies = append(proxies, proxyForm)
			proxyList.Add(proxyForm)
		})

		// 使用 container.NewVScroll 来实现滚动效果
		form := dialog.NewCustomConfirm("新建配置", "保存", "取消", container.NewVScroll(
			container.NewVBox(
				widget.NewLabel("服务器配置项"),
				serverAddr,
				serverPort,
				authToken,
				widget.NewLabel("Visitors"),
				visitorList,
				addVisitorButton,
				widget.NewLabel("Proxies"),
				proxyList,
				addProxyButton,
				errorLabel,
			),
		), func(confirm bool) {
			if !confirm {
				return
			}
			if !validateIP(serverAddr.Text) {
				markInvalid(serverAddr, false, "无效的服务器地址")
				return
			}
			if !validatePort(serverPort.Text) {
				markInvalid(serverPort, false, "端口范围应为 0-65535")
				return
			}

			// 将 IP 地址中的中间部分数字替换为 *
			maskedAddr := maskIP(serverAddr.Text)
			// 保存配置文件
			fileName := "config_" + maskedAddr + ".toml"
			content := fmt.Sprintf(`serverAddr = "%s"
serverPort = %s
auth.method = "token"
auth.token = "%s"

`, serverAddr.Text, serverPort.Text, authToken.Text)
			for _, visitor := range visitors {
				visitorForm := visitor.(*fyne.Container).Objects
				content += fmt.Sprintf(`[[visitors]]
name = "%s"
type = "%s"
serverName = "%s"
secretKey = "%s"
bindAddr = "%s"
bindPort = %s
keepTunnelOpen = %t

`,
					visitorForm[1].(*widget.Entry).Text,
					visitorForm[2].(*widget.Select).Selected,
					visitorForm[3].(*widget.Entry).Text,
					visitorForm[4].(*widget.Entry).Text,
					visitorForm[5].(*widget.Entry).Text,
					visitorForm[6].(*widget.Entry).Text,
					visitorForm[7].(*widget.Check).Checked,
				)
			}
			for _, proxy := range proxies {
				proxyForm := proxy.(*fyne.Container).Objects
				content += fmt.Sprintf(`[[proxies]]
name = "%s"
type = "%s"
localIP = "%s"
localPort = %s
`,
					proxyForm[1].(*widget.Entry).Text,
					proxyForm[2].(*widget.Select).Selected,
					proxyForm[3].(*widget.Entry).Text,
					proxyForm[4].(*widget.Entry).Text,
				)
				if proxyForm[2].(*widget.Select).Selected != "xtcp" {
					content += fmt.Sprintf("remotePort = %s\n", proxyForm[5].(*widget.Entry).Text)
				}
				if proxyForm[2].(*widget.Select).Selected == "xtcp" {
					content += fmt.Sprintf("secretKey = \"%s\"\n", proxyForm[6].(*widget.Entry).Text)
				}
				content += "\n"
			}

			err := os.WriteFile(filepath.Join(srcDir, fileName), []byte(content), 0600)
			if err != nil {
				dialog.ShowError(fmt.Errorf("保存配置文件失败: %v", err), window)
				return
			}
			refreshConfigFiles()
		}, window)
		form.Resize(fyne.NewSize(700, 500)) // 调整添加配置窗口的尺寸
		form.Show()
	})

	// 切换主题的按钮
	switchThemeButton := widget.NewButton("切换主题", func() {
		if isDarkMode {
			myApp.Settings().SetTheme(theme.LightTheme()) // 设置白天模式
		} else {
			myApp.Settings().SetTheme(theme.DarkTheme()) // 设置黑夜模式
		}
		isDarkMode = !isDarkMode
	})
	// 修改配置
	modifyConfig := func(fileName string) {
		filePath := filepath.Join(srcDir, fileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取配置文件失败: %v", err), window)
			return
		}
		entry := widget.NewMultiLineEntry()
		entry.SetText(string(content))
		dlg := dialog.NewCustomConfirm("修改配置", "保存", "取消", entry, func(confirm bool) {
			if confirm {
				err := os.WriteFile(filePath, []byte(entry.Text), 0600)
				if err != nil {
					dialog.ShowError(fmt.Errorf("保存配置文件失败: %v", err), window)
					return
				}
				refreshConfigFiles()
			}
		}, window)
		dlg.Resize(fyne.NewSize(700, 500)) // 调整修改配置窗口的尺寸
		dlg.Show()
	}

	// 删除配置
	deleteConfig := func(fileName string) {
		dlg := dialog.NewConfirm("删除配置", "确定要删除该配置文件吗？", func(confirm bool) {
			if confirm {
				err := os.Remove(filepath.Join(srcDir, fileName))
				if err != nil {
					dialog.ShowError(fmt.Errorf("删除配置文件失败: %v", err), window)
					return
				}
				refreshConfigFiles()
			}
		}, window)
		dlg.Show()
	}

	// 启动和停止 FRP
	startFRP := func() {
		if selectedID < 0 || selectedID >= len(configFiles) {
			dialog.ShowInformation("提示", "请先选择一个配置文件", window)
			return
		}

		// 获取当前工作目录
		dir, err := os.Getwd()
		if err != nil {
			logs.SetText(fmt.Sprintf("获取当前目录失败: %v", err))
			return
		}

		// 构建frpc.exe的完整路径
		configPath := filepath.Join(dir, "src", configFiles[selectedID])

		// 执行frpc.exe，传递配置文件路径
		cmd := exec.Command(filepath.Join(dir, "src", "frpc_auto.exe"), "-c", configPath)
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		// 打开日志文件
		var logFile *os.File
		if overwriteLogs {
			logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		} else {
			logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		if err != nil {
			logs.SetText(fmt.Sprintf("无法打开日志文件: %v", err))
			return
		}
		defer logFile.Close()

		// 日志协程
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				updateLogDisplay(logs, line)     // 更新界面上的日志，仅保留最新 10 行
				logFile.WriteString(line + "\n") // 写入日志文件
			}
		}()
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				updateLogDisplay(logs, line)     // 更新界面上的日志，仅保留最新 10 行
				logFile.WriteString(line + "\n") // 写入日志文件
			}
		}()
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 隐藏控制台窗口

		err = cmd.Start()
		if err != nil {
			logs.SetText(fmt.Sprintf("启动 FRP 失败: %v", err))
			return
		}

		frpProcess = cmd // 保存进程对象

		go func() {
			err = cmd.Wait()
			if err != nil {
				logs.SetText(fmt.Sprintf("FRP 运行中断: %v", err))
			} else {
				logs.SetText(logs.Text + "\nFRP 已成功运行")
			}
		}()

		logs.SetText(logs.Text + "\nFRP 已启动...")
	}

	stopFRP := func() {
		if frpProcess == nil {
			logs.SetText(logs.Text + "\n没有运行中的 FRP 进程")
			return
		}

		// 强制杀死进程
		err := frpProcess.Process.Kill()
		if err != nil {
			logs.SetText(fmt.Sprintf("强制停止 FRP 失败: %v", err))
		} else {
			logs.SetText(logs.Text + "\nFRP 已停止")
		}

		// 清理进程
		frpProcess = nil
	}

	// 配置操作按钮
	configActions := container.NewVBox(
		widget.NewButton("修改配置", func() {
			if selectedID >= 0 && selectedID < len(configFiles) {
				modifyConfig(configFiles[selectedID])
			} else {
				dialog.ShowInformation("提示", "请先选择一个配置文件", window)
			}
		}),
		widget.NewButton("删除配置", func() {
			if selectedID >= 0 && selectedID < len(configFiles) {
				deleteConfig(configFiles[selectedID])
			} else {
				dialog.ShowInformation("提示", "请先选择一个配置文件", window)
			}
		}),
	)
	// 导入配置按钮
	importConfigButton := widget.NewButton("导入配置", func() {
		// 创建选择文件按钮：导入配置文件
		selectFileButton := widget.NewButton("导入配置文件", func() {
			// 打开文件选择器
			openFileDialog := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
				if err != nil {
					dialog.ShowError(fmt.Errorf("选择文件失败: %v", err), window)
					return
				}
				if uc != nil {
					// 选择到文件后，读取文件内容并保存到src目录
					content, err := os.ReadFile(uc.URI().Path())
					if err != nil {
						dialog.ShowError(fmt.Errorf("读取文件失败: %v", err), window)
						return
					}

					// 确保src目录存在
					if _, err := os.Stat("src"); os.IsNotExist(err) {
						err := os.Mkdir("src", 0755)
						if err != nil {
							dialog.ShowError(fmt.Errorf("创建src目录失败: %v", err), window)
							return
						}
					}

					// 保存文件到src目录
					dstPath := filepath.Join("src", filepath.Base(uc.URI().Path()))
					err = os.WriteFile(dstPath, content, 0600)
					if err != nil {
						dialog.ShowError(fmt.Errorf("保存文件失败: %v", err), window)
						return
					}
					refreshConfigFiles()
					dialog.ShowInformation("成功", "配置文件导入成功", window)
				}
			}, window)
			openFileDialog.Show()
		})

		// 创建一个输入框来输入Base64字符串
		importText := widget.NewMultiLineEntry()
		importText.SetPlaceHolder("请粘贴Base64编码内容...")
		importText.Wrapping = fyne.TextWrapWord // 启用换行

		// 将 MultiLineEntry 包裹在滚动容器中，以避免超出界面
		scrollImportText := container.NewScroll(importText)
		scrollImportText.SetMinSize(fyne.NewSize(700, 300)) // 设置最小尺寸

		// 创建 Base64 导入按钮
		importBase64Button := widget.NewButton("导入Base64", func() {
			// 获取用户输入的Base64字符串
			base64Str := importText.Text
			if base64Str == "" {
				dialog.ShowError(fmt.Errorf("请输入Base64编码内容"), window)
				return
			}

			// 尝试解码Base64字符串
			decodedData, err := base64.StdEncoding.DecodeString(base64Str)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Base64解码失败: %v", err), window)
				return
			}

			// 确保src目录存在
			if _, err := os.Stat("src"); os.IsNotExist(err) {
				err := os.Mkdir("src", 0755)
				if err != nil {
					dialog.ShowError(fmt.Errorf("创建src目录失败: %v", err), window)
					return
				}
			}

			// 生成文件路径并保存解码后的文件
			dstPath := filepath.Join("src", "config.toml") // 默认保存为config.toml
			err = os.WriteFile(dstPath, decodedData, 0600)
			if err != nil {
				dialog.ShowError(fmt.Errorf("保存解码文件失败: %v", err), window)
				return
			}
			refreshConfigFiles()
			dialog.ShowInformation("成功", "Base64配置导入成功", window)
		})

		// 显示导入配置对话框
		importDialog := container.NewVBox(
			widget.NewLabel("请选择导入方式"),
			selectFileButton,
			widget.NewLabel("或者输入Base64编码内容"),
			scrollImportText,
			importBase64Button,
		)

		dialog.ShowCustom("导入配置", "关闭", importDialog, window)
	})

	exportConfigButton := widget.NewButton("导出配置", func() {
		if selectedID >= 0 && selectedID < len(configFiles) {
			// 读取文件内容
			fileContent, err := os.ReadFile(filepath.Join(srcDir, configFiles[selectedID]))
			if err != nil {
				dialog.ShowError(fmt.Errorf("读取文件失败: %v", err), window)
				return
			}
			encoded := base64.StdEncoding.EncodeToString(fileContent)

			// 创建 MultiLineEntry 并设置文本
			exportText := widget.NewMultiLineEntry()
			exportText.SetText(encoded)
			exportText.Wrapping = fyne.TextWrapWord // 启用换行
			exportText.SetPlaceHolder("Base64 编码内容...")

			// 将 MultiLineEntry 包裹在滚动容器中，以避免超出界面
			scrollExportText := container.NewScroll(exportText)
			scrollExportText.SetMinSize(fyne.NewSize(700, 300)) // 设置最小尺寸

			// 创建并显示导出对话框
			exportDialog := container.NewVBox(
				widget.NewLabel("Base64 编码内容"),
				scrollExportText,
				widget.NewButton("导出到文件", func() {
					// 导出文件的逻辑
					saveDialog := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
						if err != nil {
							dialog.ShowError(fmt.Errorf("保存文件失败: %v", err), window)
							return
						}
						if uc != nil {
							// 保存Base64内容到文件
							err := os.WriteFile(uc.URI().Path(), []byte(encoded), 0600)
							if err != nil {
								dialog.ShowError(fmt.Errorf("保存Base64编码文件失败: %v", err), window)
								return
							}
						}
					}, window)
					saveDialog.Show()
				}),
			)

			// 弹出导出配置对话框
			dialog.ShowCustom("导出配置", "关闭", exportDialog, window)
		} else {
			dialog.ShowInformation("提示", "请先选择一个配置文件", window)
		}
	})

	// 布局
	leftPanel := container.NewVBox(
		addConfigButton,
		importConfigButton,
		exportConfigButton,
		configActions,
		widget.NewButton("启动 FRP", startFRP),
		widget.NewButton("停止 FRP", stopFRP),
		switchThemeButton,
		widget.NewLabel("  powered by Deepsea"),
	)
	listAndLogs := container.NewVSplit(configList, logs)
	listAndLogs.SetOffset(0.5) // 上下平分

	mainLayout := container.NewHSplit(leftPanel, listAndLogs)
	mainLayout.SetOffset(0.3)

	window.SetContent(mainLayout)
	window.ShowAndRun()
}

func maskIP(ip string) string {
	// 假设IP地址是由四个数组组成，中间的第三个数字需要被隐藏
	// 例如：192.168.123.456 -> 192.168.***.456
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		// 如果IP格式不对，返回原IP
		return ip
	}
	// 替换中间部分
	parts[2] = "^^^"
	return strings.Join(parts, ".")
}

// 加载 .ico 文件为 Fyne 支持的图像资源
func loadIconFromFile(path string) (fyne.Resource, error) {
	// 打开 .ico 文件
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 使用 Fyne 的资源加载工具来读取图标文件
	resource, err := fyne.LoadResourceFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("加载资源失败: %v", err)
	}

	return resource, nil
}

// 初始化目录
func initDirs() error {
	err := os.MkdirAll(srcDir, 0700)
	if err != nil {
		return fmt.Errorf("无法创建 src 目录: %v", err)
	}
	return nil
}
