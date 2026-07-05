// tresbbs-server is the main BBS server.
//
// It starts a Telnet server and optionally an SSH server, handling BBS
// sessions using the ports-and-adapters architecture.
//
// Usage:
//
//	tresbbs-server [flags]
//	  -addr string      Listen address (default ":2323")
//	  -db string         Database path (default "tribbs.db")
//	  -ssh string        SSH listen address (e.g., ":2222", empty to disable)
//	  -hostkey string    SSH host key path (default "ssh_host_key")
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jasondostal/tresbbs/adapter/ansi"
	sshadapter "github.com/jasondostal/tresbbs/adapter/ssh"
	"github.com/jasondostal/tresbbs/adapter/sqlite"
	"github.com/jasondostal/tresbbs/adapter/telnet"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/event"
	"github.com/jasondostal/tresbbs/internal/filescan"
	"github.com/jasondostal/tresbbs/internal/legacy"
	nodemgr "github.com/jasondostal/tresbbs/internal/node"
	"github.com/jasondostal/tresbbs/internal/session"
)

func main() {
	var (
		addr     = flag.String("addr", ":2323", "Telnet listen address")
		dbPath   = flag.String("db", "tribbs.db", "SQLite database path")
		sshAddr   = flag.String("ssh", "", "SSH listen address (e.g. :2222)")
		hostKey   = flag.String("hostkey", "ssh_host_key", "SSH host key path")
		menuDir   = flag.String("menus", "", "Directory of TriBBS .MNU menu files (a dropped-in NWORK/); empty uses the built-in stock menus")
		importDir = flag.String("import", "", "Import an original TriBBS data directory into -db, then exit")
		scanFiles = flag.Bool("scanfiles", false, "Scan every file area's directory for new files, add them to the file base, then exit")
	)
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("tribbs v%s starting...", domain.Version)

	// Initialize storage
	storage, err := sqlite.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open storage: %v", err)
	}
	defer storage.Close()
	log.Printf("Storage: %s", *dbPath)

	// Legacy import mode: pull an original TriBBS data directory into the DB and exit.
	if *importDir != "" {
		log.Printf("Importing TriBBS data from %s ...", *importDir)
		res, err := legacy.Import(*importDir, storage)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		fmt.Println(res.String())
		return
	}

	// Scan file-area directories for new files and exit.
	if *scanFiles {
		total, byArea, err := filescan.ScanAll(storage)
		if err != nil {
			log.Fatalf("File scan failed: %v", err)
		}
		fmt.Printf("Scanned file areas: %d new file(s) added\n", total)
		for area, n := range byArea {
			if n > 0 {
				fmt.Printf("  %-24s %d\n", area, n)
			}
		}
		return
	}

	// Load or create config
	config, err := storage.GetConfig()
	if err != nil {
		log.Printf("Creating default config...")
		config = &domain.Config{
			BoardName:        "TresBBS",
			SysopName:        "Sysop",
			MaxNodes:         4,
			MaxBaud:          14400,
			NewUserSecurity:  10,
			SysopSecurity:    90,
			MaxTimePerLogon:  60,
			NewUserTimeLimit: 30,
			AllowAliases:     true,
			Allow2400Baud:    true,
			DefaultProtocol:  'Z',
			BBSStartDate:     time.Now().Format("01/02/2006"),
		}
		storage.SaveConfig(config)
	}
	log.Printf("Board: %s (%s)", config.BoardName, config.SysopName)

	// Initialize node manager
	nodes := nodemgr.NewManager(config.MaxNodes)
	log.Printf("Nodes: %d available", config.MaxNodes)

	// Create default conferences if none exist
	confs, _ := storage.GetConferences()
	if len(confs) == 0 {
		log.Printf("Creating default conferences...")
		storage.AddPost(&domain.Post{Conference: "General", Author: "Sysop", Subject: "Welcome!", Body: "Welcome to the BBS!"})
	}

	// Initialize event scheduler
	eventScheduler := event.NewScheduler(storage)
	log.Println("Event scheduler: initialized")

	// Start Telnet server
	telnetServer, err := telnet.NewServer(*addr)
	if err != nil {
		log.Fatalf("Failed to start telnet server: %v", err)
	}
	defer telnetServer.Close()
	log.Printf("Telnet: listening on %s", *addr)
	log.Printf("Connect: telnet localhost%s", *addr)

	// Start SSH server if configured
	var sshServer *sshadapter.Server
	if *sshAddr != "" {
		var sshErr error
		sshServer, sshErr = sshadapter.NewServer(*sshAddr, *hostKey, storage)
		if sshErr != nil {
			log.Fatalf("Failed to start SSH server: %v", sshErr)
		}
		defer sshServer.Close()
		log.Printf("SSH: listening on %s", sshServer.Addr())
		log.Printf("Connect: ssh localhost%s", sshServer.Addr())
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start event scheduler in background
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := eventScheduler.CheckAndExecute(); err != nil {
					log.Printf("Event check error: %v", err)
				}
			case <-sigCh:
				return
			}
		}
	}()

	// Accept loop for Telnet
	go func() {
		for {
			conn, err := telnetServer.Accept()
			if err != nil {
				select {
				case <-sigCh:
					return
				default:
				}
				log.Printf("Telnet accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			log.Printf("Telnet connection from %s", conn.RemoteAddr())

			// Assign a node number
			node := nodes.AllocNode()
			if node == 0 {
				conn.Display().WriteLine("All nodes busy. Try again later.")
				conn.Close()
				continue
			}

			// Run the BBS session in a goroutine
			go func() {
				defer nodes.FreeNode(node)
				defer conn.Close()

				// Create the session with all adapters
				display := ansi.New(conn.Reader(), conn.Writer())
				sess := session.New(display, storage, nodes, config, node, "Telnet", *menuDir, conn.RemoteAddr())
				sess.Run()

				log.Printf("Telnet disconnected: %s", conn.RemoteAddr())
			}()
		}
	}()

	// Accept loop for SSH
	if sshServer != nil {
		go func() {
			for {
				conn, err := sshServer.Accept()
				if err != nil {
					select {
					case <-sigCh:
						return
					default:
					}
					log.Printf("SSH accept error: %v", err)
					time.Sleep(100 * time.Millisecond)
					continue
				}

				log.Printf("SSH connection from %s", conn.RemoteAddr())

				// Assign a node number
				node := nodes.AllocNode()
				if node == 0 {
					conn.Display().WriteLine("All nodes busy. Try again later.")
					conn.Close()
					continue
				}

				// Run the BBS session in a goroutine
				go func() {
					defer nodes.FreeNode(node)
					defer conn.Close()

					// Create the session with all adapters
					display := ansi.New(conn.Reader(), conn.Writer())
					sess := session.New(display, storage, nodes, config, node, "SSH", *menuDir, conn.RemoteAddr())
					sess.Run()

					log.Printf("SSH disconnected: %s", conn.RemoteAddr())
				}()
			}
		}()
	}

	// Wait for shutdown signal
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
	fmt.Println("\nGoodbye!")
}

// (node management now lives in internal/node.Manager — see nodemgr above)
