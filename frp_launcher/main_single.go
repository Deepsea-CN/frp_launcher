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
)

// 嵌入 frpc.exe 二进制文件
//
//go:embed frpc.exe
var frpcBinary []byte

// 对称加密的密钥
var symmKey = []byte("frp_connect_password#769") // 密码必须是 16/24/32 字节长度（AES-128/192/256）

// AES 加密
func encryptAES(data string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("AES 创建加密块失败: %v", err)
	}

	// 填充数据以适应 AES 块大小 (16字节)
	dataBytes := []byte(data)
	blockSize := block.BlockSize()
	padding := blockSize - len(dataBytes)%blockSize
	paddedData := append(dataBytes, bytes.Repeat([]byte{byte(padding)}, padding)...)

	// 随机生成 IV (初始化向量)
	iv := make([]byte, blockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("生成随机 IV 失败: %v", err)
	}

	// 创建 CBC 模式
	cbc := cipher.NewCBCEncrypter(block, iv)

	// 加密数据
	encrypted := make([]byte, len(paddedData))
	cbc.CryptBlocks(encrypted, paddedData)

	// 将 IV 和加密数据一起返回（IV + 加密数据）
	return base64.StdEncoding.EncodeToString(append(iv, encrypted...)), nil
}

// AES 解密
func decryptAES(encoded string, key []byte) (string, error) {
	// 解码 Base64 编码的数据
	encData, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	// 提取 IV 和加密数据
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

	// 创建 CBC 模式解密器
	cbc := cipher.NewCBCDecrypter(block, iv)

	// 解密数据
	decrypted := make([]byte, len(encryptedData))
	cbc.CryptBlocks(decrypted, encryptedData)

	// 去除填充
	padding := int(decrypted[len(decrypted)-1])
	decrypted = decrypted[:len(decrypted)-padding]

	return string(decrypted), nil
}

func main() {
	// 动态生成 frpc.ini 配置内容
	originalConfig := `
serverAddr = 
serverPort = 
auth.method = 
auth.token = 

[[visitors]]

`

	// 使用 AES 加密配置文件内容
	encryptedConfig, err := encryptAES(originalConfig, symmKey)
	if err != nil {
		fmt.Println("AES 加密失败:", err)
		return
	}

	// 获取程序所在目录
	programDir, err := os.Getwd()
	if err != nil {
		fmt.Println("无法获取程序所在目录:", err)
		return
	}

	// 创建 src 目录以存放 frpc.exe 和加密配置文件
	srcDir := filepath.Join(programDir, "src")
	err = os.MkdirAll(srcDir, 0700)
	if err != nil {
		fmt.Println("无法创建 src 目录:", err)
		return
	}

	// 写入加密的 frpc.toml 文件到 src 目录
	encConfigPath := filepath.Join(srcDir, "frpc.toml.enc")
	err = os.WriteFile(encConfigPath, []byte(encryptedConfig), 0600)
	if err != nil {
		fmt.Println("无法写入加密的配置文件:", err)
		return
	}

	// 写入 frpc.exe 到 src 目录
	frpcPath := filepath.Join(srcDir, "frpc.exe")
	// 假设 frpcBinary 是嵌入的二进制数据，您需要实现写入
	err = os.WriteFile(frpcPath, frpcBinary, 0700)
	if err != nil {
		fmt.Println("无法写入 frpc.exe:", err)
		return
	}

	// 确保 frpc.exe 已写入
	_, err = os.Stat(frpcPath)
	if err != nil {
		fmt.Println("无法找到 frpc.exe:", err)
		return
	}

	// 在临时目录中创建解密后的配置文件
	tempDir := os.TempDir()
	system32ConfigPath := filepath.Join(tempDir, "System32.toml")

	// 解密配置文件并写入临时目录
	decryptedConfig, err := decryptAES(encryptedConfig, symmKey)
	if err != nil {
		fmt.Println("AES 配置解密失败:", err)
		return
	}

	err = os.WriteFile(system32ConfigPath, []byte(decryptedConfig), 0600)
	if err != nil {
		fmt.Println("无法写入解密后的配置文件:", err)
		return
	}

	// 启动 frpc.exe 时传递解密的配置文件路径
	cmd := exec.Command(frpcPath, "-c", system32ConfigPath)

	// 使用 bufio.Scanner 过滤日志输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("无法创建标准输出管道:", err)
		return
	}

	// 启动一个 goroutine 读取并处理输出
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()

			// 过滤掉包含配置文件路径的行
			if !strings.Contains(line, "System32.toml") {
				// 仅输出不包含配置文件路径的日志
				fmt.Println(line)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("日志扫描出错:", err)
		}
	}()

	// 启动 frpc.exe
	err = cmd.Start()
	if err != nil {
		fmt.Println("无法启动 frpc.exe:", err)
		return
	}

	fmt.Println("frpc.exe 已启动，按 Ctrl+C 退出。")

	// 程序退出时删除解密后的配置文件
	defer os.Remove(system32ConfigPath)

	// 等待 frpc.exe 运行完成
	err = cmd.Wait()
	if err != nil {
		fmt.Println("frpc.exe 运行时出错:", err)
	}
}
