package adapters

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/admin/message-router/internal/types"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

type TelegramAdapter struct {
	apiID   int
	apiHash string
	session string
}

func NewTelegramAdapter(apiID int, apiHash string, session string) *TelegramAdapter {
	return &TelegramAdapter{
		apiID:   apiID,
		apiHash: apiHash,
		session: session,
	}
}

func (t *TelegramAdapter) GetID() string {
	return "telegram"
}

func (t *TelegramAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	var messages []types.Message

	loader := &session.StorageMemory{}
	
	if t.session != "" {
		if err := loader.StoreSession(ctx, []byte(t.session)); err != nil {
			return nil, fmt.Errorf("failed to store session: %w", err)
		}
	}

	options := telegram.Options{
		SessionStorage: loader,
		Device: telegram.DeviceConfig{
			DeviceModel:    "Desktop",
			SystemVersion:  "Windows 10",
			AppVersion:     "4.8.1",
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
		}
	}

	client := telegram.NewClient(t.apiID, t.apiHash, options)

	err := client.Run(ctx, func(ctx context.Context) error {
		api := client.API()

		dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      100,
			OffsetPeer: &tg.InputPeerEmpty{},
		})
		if err != nil {
			return err
		}

		list, ok := dialogs.AsModified()
		if !ok {
			return fmt.Errorf("could not get dialogs")
		}

		for _, dialog := range list.GetDialogs() {
			peer := dialog.GetPeer()
			// Try to convert Peer to InputPeer
			var inputPeer tg.InputPeerClass
			if p, ok := peer.(*tg.PeerUser); ok {
				inputPeer = &tg.InputPeerUser{UserID: p.UserID}
			} else if p, ok := peer.(*tg.PeerChat); ok {
				inputPeer = &tg.InputPeerChat{ChatID: p.ChatID}
			} else if p, ok := peer.(*tg.PeerChannel); ok {
				inputPeer = &tg.InputPeerChannel{ChannelID: p.ChannelID}
			}

			if inputPeer == nil {
				continue
			}
			
			history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:  inputPeer,
				Limit: 20,
			})
			if err != nil {
				continue
			}

			msgs, ok := history.AsModified()
			if !ok {
				continue
			}

			for _, m := range msgs.GetMessages() {
				msg, ok := m.AsNotEmpty()
				if !ok {
					continue
				}

				msgObj, ok := msg.(*tg.Message)
				if !ok {
					continue
				}

				msgTime := time.Unix(int64(msgObj.Date), 0)
				if msgTime.Before(since) {
					continue
				}

				messages = append(messages, types.Message{
					ID:        strconv.Itoa(msgObj.ID),
					Source:    "telegram",
					Sender:    fmt.Sprintf("%v", msgObj.FromID),
					Content:   msgObj.Message,
					Timestamp: msgTime,
					ChatName:  fmt.Sprintf("%v", peer),
				})
			}
		}
		return nil
	})
	return messages, err
}
