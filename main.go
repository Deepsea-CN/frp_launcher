package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

var (
	srcDir      = "./src"
	configFiles []string
	selectedID  = -1 // 当前选中的配置文件索引
)

func main() {
	myApp := app.New()
	window := myApp.NewWindow("FRP 控制器")
	window.Resize(fyne.NewSize(800, 600))

	// 初始化目录
	err := initDirs()
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
	logs.Disable() // 使日志框不可编辑
	logs.Wrapping = fyne.TextWrapWord

	// 添加配置
	addConfigButton := widget.NewButton("新建配置", func() {
		serverAddr := widget.NewEntry()
		serverAddr.SetPlaceHolder("服务器地址")
		serverPort := widget.NewEntry()
		serverPort.SetPlaceHolder("服务器端口")
		authToken := widget.NewEntry()
		authToken.SetPlaceHolder("鉴权 Token")

		visitors := []fyne.CanvasObject{}
		visitorList := container.NewVBox()
		addVisitorButton := widget.NewButton("添加 Visitor", func() {
			visitorName := widget.NewEntry()
			visitorName.SetPlaceHolder("Visitor 名称")
			visitorType := widget.NewEntry()
			visitorType.SetPlaceHolder("类型")
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
			visitors = append(visitors, visitorForm)
			visitorList.Add(visitorForm)
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
			),
		), func(confirm bool) {
			if !confirm {
				return
			}
			// 保存配置文件
			fileName := "config_" + serverAddr.Text + ".toml"
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
					visitorForm[2].(*widget.Entry).Text,
					visitorForm[3].(*widget.Entry).Text,
					visitorForm[4].(*widget.Entry).Text,
					visitorForm[5].(*widget.Entry).Text,
					visitorForm[6].(*widget.Entry).Text,
					visitorForm[7].(*widget.Check).Checked,
				)
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

		// 日志协程
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				logs.SetText(logs.Text + "\n" + scanner.Text())
			}
		}()
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				logs.SetText(logs.Text + "\n" + scanner.Text())
			}
		}()

		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 隐藏控制台窗口
		err = cmd.Start()
		if err != nil {
			logs.SetText(fmt.Sprintf("启动 FRP 失败: %v", err))
			return
		}
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
		err := exec.Command("taskkill", "/F", "/IM", "frpc_auto.exe").Run()
		if err != nil {
			logs.SetText(fmt.Sprintf("停止 FRP 失败: %v", err))
		} else {
			logs.SetText(logs.Text + "\nFRP 已停止")
		}
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

	// 布局
	leftPanel := container.NewVBox(
		addConfigButton,
		configActions,
		widget.NewButton("启动 FRP", startFRP),
		widget.NewButton("停止 FRP", stopFRP),
	)
	listAndLogs := container.NewVSplit(configList, logs)
	listAndLogs.SetOffset(0.5) // 上下平分

	mainLayout := container.NewHSplit(leftPanel, listAndLogs)
	mainLayout.SetOffset(0.3)

	window.SetContent(mainLayout)
	window.ShowAndRun()
}

// 初始化目录
func initDirs() error {
	err := os.MkdirAll(srcDir, 0700)
	if err != nil {
		return fmt.Errorf("无法创建 src 目录: %v", err)
	}
	return nil
}
