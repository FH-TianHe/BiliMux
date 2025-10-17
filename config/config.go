package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	Cookie string `json:"cookie"`
	Buvid3 string `json:"buvid3"`
	Buvid4 string `json:"buvid4"`
	BNut   string `json:"b_nut"`
}

var (
	config     Config
	configLock sync.RWMutex
)

func LoadConfig(filePath string) error {
	configLock.Lock()
	defer configLock.Unlock()

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		config = Config{}
		if err := saveConfig(filePath); err != nil {
			return err
		}
		return nil
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &config)
}

func SaveConfig(filePath string) error {
	configLock.Lock()
	defer configLock.Unlock()
	return saveConfig(filePath)
}

func saveConfig(filePath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, data, 0644)
}

func GetConfig() Config {
	configLock.RLock()
	defer configLock.RUnlock()
	return config
}

func SetCookie(cookie string) {
	configLock.Lock()
	defer configLock.Unlock()
	config.Cookie = cookie
}

func SetBuvid3(buvid3 string) {
	configLock.Lock()
	defer configLock.Unlock()
	config.Buvid3 = buvid3
}

func SetBuvid4(buvid4 string) {
	configLock.Lock()
	defer configLock.Unlock()
	config.Buvid4 = buvid4
}

func SetBNut(bNut string) {
	configLock.Lock()
	defer configLock.Unlock()
	config.BNut = bNut
}
