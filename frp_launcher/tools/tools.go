package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

var symmKey = []byte("frp_connect_password#769")

func encryptAES(data string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padding := block.BlockSize() - len(data)%block.BlockSize()
	paddedData := append([]byte(data), bytes.Repeat([]byte{byte(padding)}, padding)...)
	iv := make([]byte, block.BlockSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	encrypted := make([]byte, len(paddedData))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, paddedData)
	return base64.StdEncoding.EncodeToString(append(iv, encrypted...)), nil
}

func decryptAES(encoded string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := data[:block.BlockSize()]
	decrypted := make([]byte, len(data)-block.BlockSize())
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, data[block.BlockSize():])
	padding := int(decrypted[len(decrypted)-1])
	return string(decrypted[:len(decrypted)-padding]), nil
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("\n=== 加密解密工具 ===")
		fmt.Println("1. 加密配置文件")
		fmt.Println("2. 解密配置文件")
		fmt.Println("3. 退出")
		fmt.Print("选择操作: ")
		input, _ := reader.ReadString('\n')
		switch strings.TrimSpace(input) {
		case "1":
			fmt.Print("输入配置文件路径: ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Println("读取文件失败:", err)
				continue
			}
			encrypted, err := encryptAES(string(data), symmKey)
			if err != nil {
				fmt.Println("加密失败:", err)
				continue
			}
			outputPath := path + ".enc"
			err = os.WriteFile(outputPath, []byte(encrypted), 0600)
			if err != nil {
				fmt.Println("保存加密文件失败:", err)
			} else {
				fmt.Println("加密成功，保存到:", outputPath)
			}
		case "2":
			fmt.Print("输入加密文件路径: ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Println("读取文件失败:", err)
				continue
			}
			decrypted, err := decryptAES(string(data), symmKey)
			if err != nil {
				fmt.Println("解密失败:", err)
				continue
			}
			outputPath := strings.TrimSuffix(path, ".enc") + ".dec"
			err = os.WriteFile(outputPath, []byte(decrypted), 0600)
			if err != nil {
				fmt.Println("保存解密文件失败:", err)
			} else {
				fmt.Println("解密成功，保存到:", outputPath)
			}
		case "3":
			fmt.Println("退出工具。")
			return
		default:
			fmt.Println("无效输入，请重试。")
		}
	}
}
