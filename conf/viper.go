package conf

import (
	"fmt"

	"github.com/spf13/viper"
)

var globalConfig *viper.Viper
var isDevMode = false
var enableDbDebug = false

func GetConfig() *viper.Viper {
	if globalConfig == nil {
		initConfig()
	}
	return globalConfig
}

func initConfig() {
	vc := viper.New()
	vc.AddConfigPath("./")
	err := vc.ReadInConfig()
	if err != nil {
		fmt.Println("致命错误: 读取配置文件失败，触发panic", err)
		panic("致命错误: 读取配置文件失败，触发panic" + err.Error())
	}
	globalConfig = vc
	isDevMode = vc.GetBool("app.dev_mode")
	enableDbDebug = vc.GetBool("app.enable_db_debug")
}

func IsDevMode() bool {
	return isDevMode
}

func EnableDBDebug() bool {
	return enableDbDebug
}
