package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/session/tdesktop"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Telegram Session Generator")
	fmt.Println("--------------------------")
	fmt.Println("1. Use Official Android (API ID: 6)")
	fmt.Println("2. Use Official Desktop (API ID: 2040)")
	fmt.Println("3. Use Official Web (API ID: 2496)")
	fmt.Println("4. Use Telegram X (API ID: 21724)")
	fmt.Println("5. Import from local Telegram Desktop (macOS)")
	fmt.Println("6. Use Custom API ID")
	fmt.Print("Select option [1-6]: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "5" {
		importFromTDesktop(reader)
		return
	}

	var apiID int
	var apiHash string
	var deviceModel string
	var systemVersion string

	switch choice {
	case "1":
		apiID = 6
		apiHash = "eb06d4ab3525102a0a256a480575d507"
		deviceModel = "Samsung SM-G998B"
		systemVersion = "Android 13"
	case "2":
		apiID = 2040
		apiHash = "b18441a5434e907746766487968e778a"
		deviceModel = "Desktop"
		systemVersion = "Windows 11"
	case "3":
		apiID = 2496
		apiHash = "8da85b0d5b9701665114cc554605915c"
		deviceModel = "Web"
		systemVersion = "Chrome/122.0.0.0"
	case "4":
		apiID = 21724
		apiHash = "3e0cb6511003ca3ad4fda2d87027c51d"
		deviceModel = "Telegram X"
		systemVersion = "Android 12"
	case "6":
		fmt.Print("Enter Custom API ID: ")
		customID, _ := reader.ReadString('\n')
		fmt.Sscanf(strings.TrimSpace(customID), "%d", &apiID)
		fmt.Print("Enter Custom API Hash: ")
		apiHash, _ = reader.ReadString('\n')
		apiHash = strings.TrimSpace(apiHash)
		deviceModel = "Desktop"
		systemVersion = "Windows 11"
	default:
		log.Fatal("Invalid choice")
	}

	fmt.Print("Enter Phone Number (e.g., +1234567890): ")
	phone, _ := reader.ReadString('\n')
	phone = strings.TrimSpace(phone)

	ctx := context.Background()
	storage := &session.StorageMemory{}

	options := telegram.Options{
		SessionStorage: storage,
		Device: telegram.DeviceConfig{
			DeviceModel:    deviceModel,
			SystemVersion:  systemVersion,
			AppVersion:     "10.5.0",
			SystemLangCode: "en",
			LangCode:       "en",
		},
	}

	if proxyAddr := os.Getenv("ALL_PROXY"); proxyAddr != "" {
		dialer, err := proxy.SOCKS5("tcp", strings.TrimPrefix(proxyAddr, "socks5://"), nil, proxy.Direct)
		if err == nil {
			options.Resolver = dcs.Plain(dcs.PlainOptions{
				Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			})
			fmt.Printf("Using SOCKS5 proxy: %s\n", proxyAddr)
		} else {
			log.Printf("Warning: Failed to create SOCKS5 proxy: %v", err)
		}
	}

	client := telegram.NewClient(apiID, apiHash, options)

	codePrompt := func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
		fmt.Print("Enter Code: ")
		code, _ := reader.ReadString('\n')
		return strings.TrimSpace(code), nil
	}

	err := client.Run(ctx, func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, auth.NewFlow(
			auth.Constant(phone, "", auth.CodeAuthenticatorFunc(codePrompt)),
			auth.SendCodeOptions{},
		)); err != nil {
			return err
		}

		sessionData, err := storage.LoadSession(ctx)
		if err != nil {
			return err
		}

		fmt.Println("\n--- SUCCESS ---")
		fmt.Println("Copy this Session String into your dashboard:")
		fmt.Println(string(sessionData))
		fmt.Println("--- END ---")

		return nil
	})

	if err != nil {
		log.Fatalf("Failed to generate session: %v", err)
	}
}

func importFromTDesktop(reader *bufio.Reader) {
	fmt.Println("\nImporting from local Telegram Desktop...")
	
	home, _ := os.UserHomeDir()
	defaultPath := home + "/Library/Application Support/Telegram Desktop/tdata"
	
	fmt.Printf("Default tdata path: %s\n", defaultPath)
	fmt.Print("Press Enter to use default, or type a custom path: ")
	customPath, _ := reader.ReadString('\n')
	customPath = strings.TrimSpace(customPath)
	if customPath != "" {
		defaultPath = customPath
	}

	fmt.Print("Enter Telegram Passcode (if any, otherwise press Enter): ")
	passcode, _ := reader.ReadString('\n')
	passcode = strings.TrimSpace(passcode)

	accounts, err := tdesktop.Read(defaultPath, []byte(passcode))
	if err != nil {
		log.Fatalf("Failed to read tdata: %v", err)
	}

	if len(accounts) == 0 {
		log.Fatal("No accounts found in tdata")
	}

	// For simplicity, take the first account
	acc := accounts[0]
	
	// Convert tdesktop account to session data
	data, err := session.TDesktopSession(acc)
	if err != nil {
		log.Fatalf("Failed to convert tdesktop account to session data: %v", err)
	}
	
	// Encode data back to the format used by StorageMemory
	// We'll just marshal it for display
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("Failed to marshal session data: %v", err)
	}

	fmt.Println("\n--- SUCCESS ---")
	fmt.Println("Imported account:", acc.Authorization.UserID)
	fmt.Println("Copy this Session String into your dashboard:")
	fmt.Println(string(jsonData))
	fmt.Println("--- END ---")
}
