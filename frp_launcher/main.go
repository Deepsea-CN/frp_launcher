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
	"strings"
	"sync"
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

func encryptAES(data string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("AES 创建加密块失败: %v", err)
	}

	dataBytes := []byte(data)
	blockSize := block.BlockSize()
	padding := blockSize - len(dataBytes)%blockSize
	paddedData := append(dataBytes, bytes.Repeat([]byte{byte(padding)}, padding)...)

	iv := make([]byte, blockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("生成随机 IV 失败: %v", err)
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

func loadConfig(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("无法读取配置文件: %v", err)
	}

	data := string(content)

	if _, err := decryptAES(data, symmKey); err == nil {
		fmt.Println("检测到加密配置")
		err = os.WriteFile(encConfigPath, content, 0600)
		if err != nil {
			return fmt.Errorf("无法保存加密配置文件: %v", err)
		}
	} else {
		fmt.Println("检测到明文配置")
		encrypted, err := encryptAES(data, symmKey)
		if err != nil {
			return fmt.Errorf("配置加密失败: %v", err)
		}

		err = os.WriteFile(encConfigPath, []byte(encrypted), 0600)
		if err != nil {
			return fmt.Errorf("无法保存加密配置文件: %v", err)
		}
	}

	return nil
}

func startFrpc() error {
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
			fmt.Println(scanner.Text())
		}
	}()

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("frpc 启动失败: %v", err)
	}

	fmt.Println("frpc 已启动")
	return nil
}

func stopFrpc() error {
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
	fmt.Println("frpc 已停止")
	return nil
}

func main() {
	err := initDirs()
	if err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=== frpc 控制界面 ===")
		fmt.Println("1. 载入配置文件")
		fmt.Println("2. 启动 frpc")
		fmt.Println("3. 停止 frpc")
		fmt.Println("4. 退出")
		fmt.Print("请选择操作: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Print("请输入配置文件路径: ")
			configPath, _ := reader.ReadString('\n')
			configPath = strings.TrimSpace(configPath)
			err := loadConfig(configPath)
			if err != nil {
				fmt.Println("载入配置失败:", err)
			} else {
				fmt.Println("配置文件已成功载入")
			}
		case "2":
			err := startFrpc()
			if err != nil {
				fmt.Println("启动失败:", err)
			}
		case "3":
			err := stopFrpc()
			if err != nil {
				fmt.Println("停止失败:", err)
			}
		case "4":
			fmt.Println("退出程序")
			return
		default:
			fmt.Println("无效的选项")
		}
	}
}
