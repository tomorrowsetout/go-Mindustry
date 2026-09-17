package main

import (
	"fmt"
	"plugin"
)

func ModLoader(path string) {
	fmt.Println("开始加载.", path)
	file, err := plugin.Open(path)
	if err == nil {
		fmt.Println("加载", path, "中...")
		function, err := file.Lookup("Init")
		if err == nil {
			i, ok := function.(func())
			if ok {
				i()
				fmt.Println("成功加载", path)
			} else {
				fmt.Println(path, "Init错误", function)
			}
		} else {
			fmt.Println(path, "没有找到Init")
		}
	}
}
