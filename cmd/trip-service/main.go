package main

import (
	"fmt"
	"template/internal/validate"
)

func main() {
	Config, err := validate.LoadConfig()
	if err != nil {
		panic(err)
	}
	fmt.Println(Config.LOG_LEVEL)
}
