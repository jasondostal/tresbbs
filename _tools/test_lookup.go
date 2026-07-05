package main

import (
	"fmt"
	"github.com/jasondostal/tresbbs/adapter/sqlite"
)

func main() {
	storage, err := sqlite.New("tribbs.db")
	if err != nil {
		panic(err)
	}
	defer storage.Close()

	user, err := storage.GetUserByName("Jason")
	if err != nil {
		fmt.Printf("GetUserByName error: %v\n", err)
	} else {
		fmt.Printf("Found: %s (alias: %s, security: %d)\n", user.Name, user.Alias, user.SecurityLevel)
	}
}
