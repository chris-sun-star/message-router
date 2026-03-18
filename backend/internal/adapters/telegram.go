package adapters

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/admin/message-router/internal/config"
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

func NewTelegramClient(apiID int, apiHash string, storage session.Storage) *telegram.Client {
	options := telegram.Options{
		SessionStorage: storage,
		Device: telegram.DeviceConfig{
			DeviceModel:    "Desktop",
			SystemVersion:  "Windows 10",
			AppVersion:     "4.8.1",
			SystemLangCode: "en",
			LangCode:       "en",
		},
	}

	proxyAddr := config.AppConfig.Network.Proxy
	if proxyAddr != "" {
		dialer, err := proxy.SOCKS5("tcp", strings.TrimPrefix(proxyAddr, "socks5://"), nil, proxy.Direct)
		if err == nil {
			options.Resolver = dcs.Plain(dcs.PlainOptions{
				Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			})
		}
	}

	return telegram.NewClient(apiID, apiHash, options)
}

func (t *TelegramAdapter) GetID() string {
	return "telegram"
}

func (t *TelegramAdapter) updateNameMaps(users []tg.UserClass, chats []tg.ChatClass, userNames map[int64]string, chatNames map[int64]string) {
	for _, u := range users {
		if user, ok := u.AsNotEmpty(); ok {
			if user.FirstName != "" || user.LastName != "" {
				userNames[user.ID] = strings.TrimSpace(user.FirstName + " " + user.LastName)
			} else if user.Username != "" {
				userNames[user.ID] = user.Username
			} else {
				userNames[user.ID] = strconv.FormatInt(user.ID, 10)
			}
		}
	}
	for _, c := range chats {
		if chat, ok := c.AsNotEmpty(); ok {
			switch ch := chat.(type) {
			case *tg.Chat:
				chatNames[ch.ID] = ch.Title
			case *tg.Channel:
				chatNames[ch.ID] = ch.Title
			}
		}
	}
}

func (t *TelegramAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	var messages []types.Message

	loader := &session.StorageMemory{}

	if t.session != "" {
		if err := loader.StoreSession(ctx, []byte(t.session)); err != nil {
			return nil, fmt.Errorf("failed to store session: %w", err)
		}
	}

	client := NewTelegramClient(t.apiID, t.apiHash, loader)

	err := client.Run(ctx, func(ctx context.Context) error {
		api := client.API()

		// Get self info
		self, err := client.Self(ctx)
		var selfID int64
		var selfUsername string
		if err == nil {
			selfID = self.ID
			selfUsername = self.Username
		} else {
			return fmt.Errorf("failed to get self info: %w", err)
		}

		dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      100,
			OffsetPeer: &tg.InputPeerEmpty{},
		})
		if err != nil {
			return fmt.Errorf("failed to get dialogs: %w", err)
		}

		list, ok := dialogs.AsModified()
		if !ok {
			return fmt.Errorf("could not get dialogs")
		}

		// Pre-populate name maps from dialogs
		userNames := make(map[int64]string)
		for _, u := range list.GetUsers() {
			user, ok := u.AsNotEmpty()
			if !ok {
				continue
			}
			// user is already *tg.User since AsNotEmpty() returns it
			if user.FirstName != "" || user.LastName != "" {
				userNames[user.ID] = strings.TrimSpace(user.FirstName + " " + user.LastName)
			} else if user.Username != "" {
				userNames[user.ID] = user.Username
			} else {
				userNames[user.ID] = strconv.FormatInt(user.ID, 10)
			}
		}

		chatNames := make(map[int64]string)
		for _, c := range list.GetChats() {
			if chat, ok := c.AsNotEmpty(); ok {
				switch ch := chat.(type) {
				case *tg.Chat:
					chatNames[ch.ID] = ch.Title
				case *tg.Channel:
					chatNames[ch.ID] = ch.Title
				case *tg.ChatForbidden:
					chatNames[ch.ID] = ch.Title
				case *tg.ChannelForbidden:
					chatNames[ch.ID] = ch.Title
				}
			}
		}

		for _, dialog := range list.GetDialogs() {
			peer := dialog.GetPeer()
			
			// Detect if it's a group/channel and resolve chat name
			isGroup := false
			currentChatName := ""
			
			if p, ok := peer.(*tg.PeerChat); ok {
				isGroup = true
				currentChatName = chatNames[p.ChatID]
			} else if p, ok := peer.(*tg.PeerChannel); ok {
				isGroup = true
				currentChatName = chatNames[p.ChannelID]
			} else if p, ok := peer.(*tg.PeerUser); ok {
				currentChatName = userNames[p.UserID]
			}

			if currentChatName == "" {
				currentChatName = "Unknown"
			}

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

			// Update name maps with users/chats from history for better resolution
			switch h := history.(type) {
			case *tg.MessagesMessages:
				t.updateNameMaps(h.Users, h.Chats, userNames, chatNames)
			case *tg.MessagesMessagesSlice:
				t.updateNameMaps(h.Users, h.Chats, userNames, chatNames)
			case *tg.MessagesChannelMessages:
				t.updateNameMaps(h.Users, h.Chats, userNames, chatNames)
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

				// 1. Filter out messages sent by the account itself using Out flag
				if msgObj.Out {
					continue
				}

				senderName := "Unknown"
				if from, ok := msgObj.GetFromID(); ok {
					if p, ok := from.(*tg.PeerUser); ok {
						senderName = userNames[p.UserID]
						if senderName == "" {
							senderName = strconv.FormatInt(p.UserID, 10)
						}
					}
				}

				// Fallback for DMs if senderName is still unknown
				if senderName == "Unknown" && !isGroup {
					senderName = currentChatName
				}

				// 2. Filter group messages without mentions
				if isGroup {
					mentioned := msgObj.Mentioned || msgObj.MediaUnread
					
					// If not explicitly flagged as mentioned, check text for "@username" (case-insensitive)
					if !mentioned && selfUsername != "" {
						if strings.Contains(strings.ToLower(msgObj.Message), "@"+strings.ToLower(selfUsername)) {
							mentioned = true
						}
					}

					// If still not mentioned, check entities (just in case)
					if !mentioned {
						for _, ent := range msgObj.Entities {
							switch e := ent.(type) {
							case *tg.MessageEntityMention:
								// The mention entity doesn't contain the username itself, it just marks where it is in the text
								// We already checked the text above, but let's double check the specific mention
								start := e.Offset
								end := e.Offset + e.Length
								if start >= 0 && end <= len(msgObj.Message) {
									mentionText := strings.ToLower(msgObj.Message[start:end])
									if mentionText == "@"+strings.ToLower(selfUsername) {
										mentioned = true
										break
									}
								}
							case *tg.MessageEntityMentionName:
								if e.UserID == selfID {
									mentioned = true
									break
								}
							}
							if mentioned {
								break
							}
						}
					}

					// If still not mentioned and it's a group, ignore it
					if !mentioned {
						continue
					}
				}

				msgTime := time.Unix(int64(msgObj.Date), 0)
				// Use a small buffer (1s) to ensure we don't miss messages in the same second
				if msgTime.Before(since.Add(-1 * time.Second)) {
					continue
				}

				// Detect media
				content := msgObj.Message
				if msgObj.Media != nil {
					mediaType := ""
					switch msgObj.Media.(type) {
					case *tg.MessageMediaPhoto:
						mediaType = "[Photo]"
					case *tg.MessageMediaDocument:
						mediaType = "[File/Video/Audio]"
					case *tg.MessageMediaGeo:
						mediaType = "[Location]"
					case *tg.MessageMediaContact:
						mediaType = "[Contact]"
					case *tg.MessageMediaDice:
						mediaType = "[Dice]"
					case *tg.MessageMediaPoll:
						mediaType = "[Poll]"
					default:
						mediaType = "[Media]"
					}
					if content == "" {
						content = mediaType
					} else {
						content = mediaType + " " + content
					}
				}

				messages = append(messages, types.Message{
					ID:        strconv.Itoa(msgObj.ID),
					Source:    "telegram",
					Sender:    senderName,
					Content:   content,
					Timestamp: msgTime,
					IsPrivate: !isGroup,
					ChatName:  currentChatName,
				})
			}
		}
		return nil
	})
	return messages, err
}
