package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdp/qrterminal/v3"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	config.Load()
	
	dbPath := filepath.Join(config.StoreDir, "whatsapp.db")
	os.MkdirAll(config.StoreDir, 0755)
	ctx := context.Background()

	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), waLog.Noop)
	if err != nil {
		fmt.Printf("Failed to open whatsapp store: %v\n", err)
		os.Exit(1)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		fmt.Printf("Failed to get whatsapp device: %v\n", err)
		os.Exit(1)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Noop)

	if client.Store.ID == nil {
		// No ID stored, need to login with QR
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			fmt.Printf("Failed to connect to whatsapp: %v\n", err)
			os.Exit(1)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("WhatsApp: Scan the QR code above to login")
			} else if evt.Event == "success" {
				fmt.Println("WhatsApp: Login successful!")
			} else {
				fmt.Println("WhatsApp: Login event:", evt.Event)
			}
		}
	} else {
		fmt.Println("WhatsApp: Already logged in.")
		err = client.Connect()
		if err != nil {
			fmt.Printf("Failed to connect to whatsapp: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("WhatsApp: Connection successful.")
	}

	client.Disconnect()
	fmt.Println("Done.")
}
