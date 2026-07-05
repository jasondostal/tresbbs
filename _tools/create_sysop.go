package main

import (
	"fmt"
	"time"

	"github.com/jasondostal/tresbbs/adapter/sqlite"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
)

func main() {
	storage, err := sqlite.New("tribbs.db")
	if err != nil {
		panic(err)
	}
	defer storage.Close()

	hash, _ := auth.HashPassword("sysop")
	
	user := &domain.User{
		Name:           "Jason Dostal",
		Alias:          "Sysop",
		Password:       hash,
		City:           "Madison",
		Phone:          "555-1234",
		SecurityLevel:  99,
		ANSIMode:       1,
		ScreenWidth:    80,
		TimeLeftToday:  999,
		DailyFileLimit: 999,
		DailyByteLimit: 999999,
		Protocol:       'Z',
		Editor:         "Full",
		Email:          "jason@ducktyping.dev",
		StreetAddress:  "123 Main St",
		State:          "WI",
		ZipCode:        "53703",
		Country:        "US",
		CallsToday:     1,
		TotalCalls:     1,
		LastLogin:      time.Now(),
	}

	num, err := storage.AddUser(user)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created sysop account #%d\n", num)
	fmt.Println("Username: Sysop")
	fmt.Println("Password: sysop")
	fmt.Println("Security: 99")
}
