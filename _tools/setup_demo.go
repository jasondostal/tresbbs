// setup_demo.go seeds a fresh, playable TresBBS board.
//
//	go run _tools/setup_demo.go -db board.db
//
// Then scan the file areas and run it:
//
//	go run ./cmd/tresbbs-server -db board.db -scanfiles
//	go run ./cmd/tresbbs-server -db board.db -addr :2323
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/jasondostal/tresbbs/adapter/sqlite"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
)

func main() {
	dbPath := flag.String("db", "board.db", "database to seed")
	filesBase := flag.String("files", "/Users/jdostal/working/tresbbs/files", "file-area base directory")
	flag.Parse()

	st, err := sqlite.New(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	// Board config.
	must(st.SaveConfig(&domain.Config{
		BoardName:        "TresBBS",
		SysopName:        "Jason",
		MaxNodes:         8,
		MaxBaud:          14400,
		NewUserSecurity:  10,
		SysopSecurity:    90,
		MaxTimePerLogon:  90,
		NewUserTimeLimit: 60,
		AllowAliases:     true,
		Allow2400Baud:    true,
		DefaultProtocol:  'Z',
		BBSStartDate:     time.Now().Format("01/02/2006"),
	}))

	// Conferences.
	for _, c := range []domain.Conference{
		{Name: "General", SecurityLevel: 0},
		{Name: "Retro Computing", SecurityLevel: 0},
		{Name: "The Underground", SecurityLevel: 0},
		{Name: "Sysop Only", SecurityLevel: 90, PrivateConf: true},
	} {
		must(st.AddConference(&c))
	}

	// File areas pointing at real directories.
	for _, a := range []domain.FileArea{
		{Name: "General", Description: "Board info + odds and ends", Path: *filesBase + "/general", SortType: "name"},
		{Name: "Text Files", Description: "The good old t-files", Path: *filesBase + "/textfiles", SortType: "name"},
		{Name: "Retro", Description: "Door games + vintage computing", Path: *filesBase + "/retro", SortType: "name"},
		{Name: "ANSI Art", Description: "Drop your .ANS here", Path: *filesBase + "/ansi", SortType: "name"},
	} {
		must(st.AddFileArea(&a))
	}

	// Bulletins.
	for _, b := range []domain.Bulletin{
		{Name: "Welcome", Content: "Welcome to TresBBS! A love letter to TriBBS, running on modern iron.\nRegister, poke around the conferences, grab a file, page a node."},
		{Name: "News", Content: "This board reads original TriBBS .DAT files — drop your 1996 user\nbase in and it just works. Doors, FidoNet, and an Atari 800XL dial-in\nare on the wish list."},
	} {
		must(st.AddBulletin(&b))
	}

	// Sysop account (change the password once you're in: User Config).
	hash, _ := auth.HashPassword("letmein")
	if _, err := st.AddUser(&domain.User{
		Name: "Jason Dostal", Alias: "jason", Password: hash,
		City: "Plover, WI", SecurityLevel: 100, NodeSecurity: 100,
		Editor: "Full", Protocol: 'Z', ANSIMode: 1, ScreenWidth: 80,
	}); err != nil {
		log.Printf("(sysop user may already exist: %v)", err)
	}

	// A couple of seed posts so the conferences aren't empty.
	must(st.AddPost(&domain.Post{Conference: "General", Author: "Jason", Subject: "First post!", Body: "The board is live. Say hi, break something, report the bug."}))
	must(st.AddPost(&domain.Post{Conference: "Retro Computing", Author: "Jason", Subject: "What was your first BBS?", Body: "Mine ran on an Atari 800XL over a 300 baud modem. Beat that."}))

	fmt.Printf("Seeded %s: board=TresBBS, sysop alias=jason password=letmein\n", *dbPath)
	fmt.Println("Next: go run ./cmd/tresbbs-server -db", *dbPath, "-scanfiles")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
