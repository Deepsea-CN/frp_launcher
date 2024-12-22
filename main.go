package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

//go:embed frpc.exe
var frpcBinary []byte

var symmKey = []byte("frp_connect_password#769")

var (
	srcDir        string
	encConfigPath string
	frpcPath      string
	cmd           *exec.Cmd
	mutex         sync.Mutex
)

func initDirs() error {
	var err error

	programDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("无法获取程序所在目录: %v", err)
	}

	srcDir = filepath.Join(programDir, "src")
	err = os.MkdirAll(srcDir, 0700)
	if err != nil {
		return fmt.Errorf("无法创建 src 目录: %v", err)
	}

	encConfigPath = filepath.Join(srcDir, "frpc.toml.enc")
	frpcPath = filepath.Join(srcDir, "frpc.exe")

	err = os.WriteFile(frpcPath, frpcBinary, 0700)
	if err != nil {
		return fmt.Errorf("无法写入 frpc.exe: %v", err)
	}

	return nil
}

func encryptAES(data string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	dataBytes := []byte(data)
	blockSize := block.BlockSize()
	padding := blockSize - len(dataBytes)%blockSize
	paddedData := append(dataBytes, bytes.Repeat([]byte{byte(padding)}, padding)...)

	iv := make([]byte, blockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	cbc := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(paddedData))
	cbc.CryptBlocks(encrypted, paddedData)

	return base64.StdEncoding.EncodeToString(append(iv, encrypted...)), nil
}

func decryptAES(encoded string, key []byte) (string, error) {
	encData, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	blockSize := aes.BlockSize
	if len(encData) < blockSize {
		return "", fmt.Errorf("数据太短，无法解密")
	}
	iv := encData[:blockSize]
	encryptedData := encData[blockSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	cbc := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encryptedData))
	cbc.CryptBlocks(decrypted, encryptedData)

	padding := int(decrypted[len(decrypted)-1])
	decrypted = decrypted[:len(decrypted)-padding]

	return string(decrypted), nil
}

func loadConfig(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("无法读取配置文件: %v", err)
	}

	encrypted, err := encryptAES(string(content), symmKey)
	if err != nil {
		return fmt.Errorf("配置加密失败: %v", err)
	}

	err = os.WriteFile(encConfigPath, []byte(encrypted), 0600)
	if err != nil {
		return fmt.Errorf("无法写入加密配置文件: %v", err)
	}

	return nil
}

func startFrpc(logOutput *widget.Label) error {
	mutex.Lock()
	defer mutex.Unlock()

	if cmd != nil {
		return fmt.Errorf("frpc 已经在运行")
	}

	encData, err := os.ReadFile(encConfigPath)
	if err != nil {
		return fmt.Errorf("无法读取加密配置文件: %v", err)
	}

	decryptedConfig, err := decryptAES(string(encData), symmKey)
	if err != nil {
		return fmt.Errorf("配置解密失败: %v", err)
	}

	tempConfigPath := filepath.Join(os.TempDir(), "frpc_temp_config.toml")
	err = os.WriteFile(tempConfigPath, []byte(decryptedConfig), 0600)
	if err != nil {
		return fmt.Errorf("无法写入解密配置文件: %v", err)
	}

	cmd = exec.Command(frpcPath, "-c", tempConfigPath)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法创建标准输出管道: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			logOutput.SetText(logOutput.Text + "\n" + line)
		}
	}()

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("frpc 启动失败: %v", err)
	}

	logOutput.SetText("frpc 已启动")
	return nil
}

func stopFrpc(logOutput *widget.Label) error {
	mutex.Lock()
	defer mutex.Unlock()

	if cmd == nil {
		return fmt.Errorf("frpc 未运行")
	}

	err := cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("无法停止 frpc: %v", err)
	}

	cmd = nil
	logOutput.SetText("frpc 已停止")
	return nil
}

func main() {
	// 使用 Fyne 的软件渲染器
	//os.Setenv("FYNE_RENDERER", "software")

	err := initDirs()
	if err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	a := app.New()
	w := a.NewWindow("FRPC 控制面板")
	w.Resize(fyne.NewSize(500, 400))

	logOutput := widget.NewLabel("日志输出...")
	startButton := widget.NewButton("启动 FRPC", func() {
		err := startFrpc(logOutput)
		if err != nil {
			dialog.ShowError(err, w)
		}
	})

	stopButton := widget.NewButton("停止 FRPC", func() {
		err := stopFrpc(logOutput)
		if err != nil {
			dialog.ShowError(err, w)
		}
	})

	loadButton := widget.NewButton("加载配置文件", func() {
		dialog.ShowFileOpen(func(file fyne.URIReadCloser, _ error) {
			if file == nil {
				return
			}
			err := loadConfig(file.URI().Path())
			if err != nil {
				dialog.ShowError(err, w)
			} else {
				dialog.ShowInformation("成功", "配置文件已载入", w)
			}
		}, w)
	})

	content := container.NewVBox(
		loadButton,
		startButton,
		stopButton,
		logOutput,
	)

	w.SetContent(content)
	w.ShowAndRun()
}
