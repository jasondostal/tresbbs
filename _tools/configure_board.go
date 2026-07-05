package main

import (
	"fmt"

	"github.com/jasondostal/tresbbs/adapter/sqlite"
)

func main() {
	storage, err := sqlite.New("tresbbs.db")
	if err != nil {
		panic(err)
	}
	defer storage.Close()

	config, err := storage.GetConfig()
	if err != nil {
		panic(err)
	}

	config.BoardName = "ducktyping.dev BBS"
	config.SysopName = "Jason Dostal"
	config.SysopSecurity = 90
	config.MaxNodes = 4
	config.MaxTimePerLogon = 120
	config.NewUserTimeLimit = 30
	config.NewUserSecurity = 10
	config.AllowAliases = true
	config.DefaultCountry = "US"
	config.EnableCallerID = false // telnet, not modem
	config.BBSStartDate = "07/04/2026"

	if err := storage.SaveConfig(config); err != nil {
		panic(err)
	}

	fmt.Println("Board configured:")
	fmt.Printf("  Name: %s\n", config.BoardName)
	fmt.Printf("  Sysop: %s\n", config.SysopName)
	fmt.Printf("  Max Time: %d min\n", config.MaxTimePerLogon)
	fmt.Printf("  Nodes: %d\n", config.MaxNodes)
}
