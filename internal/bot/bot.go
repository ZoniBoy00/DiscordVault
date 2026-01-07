package bot

import (
	"bytes"
	"crypto/sha256"
	"discordvault/internal/config"
	"discordvault/internal/crypto"
	"discordvault/internal/database"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	ChunkSize = 7 * 1024 * 1024 // 7MB - Safe for all Discord servers
)

type Bot struct {
	Session *discordgo.Session
	Config  *config.Config
	DB      *database.Database
}

func New(cfg *config.Config, db *database.Database) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}

	return &Bot{
		Session: dg,
		Config:  cfg,
		DB:      db,
	}, nil
}

func (b *Bot) Start() error {
	b.Session.AddHandler(b.interactionCreate)

	err := b.Session.Open()
	if err != nil {
		return err
	}

	b.Session.UpdateGameStatus(0, "Locking away secrets... 🔒")
	log.Printf("[BOT] Online as: %v", b.Session.State.User.String())

	commands := []*discordgo.ApplicationCommand{
		{Name: "help", Description: "Show available commands"},
		{Name: "ping", Description: "Check bot latency"},
		{Name: "list", Description: "List all stored files"},
		{Name: "upload", Description: "Upload a file to the vault", Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionAttachment, Name: "file", Description: "File to upload", Required: true},
		}},
		{Name: "delete", Description: "Delete a file from the vault", Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionInteger, Name: "id", Description: "File ID", Required: true},
		}},
	}

	for _, v := range commands {
		_, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, "", v)
		if err != nil {
			log.Printf("[BOT ERR] Cannot create '%v' command: %v", v.Name, err)
		}
	}

	return nil
}

func (b *Bot) NotifyUpload(filename string, size int64, parts int, method string) {
	b.Session.ChannelMessageSend(b.Config.ChannelID, fmt.Sprintf("📤 **%s Upload Complete**\n**File:** `%s`\n**Size:** `%s`\n**Parts:** %d\n**Time:** `%s`\n**Status:** Encrypted & Locked",
		method, filename, formatBytes(size), parts, time.Now().Format("15:04:05")))
}

func (b *Bot) checkPermission(i *discordgo.InteractionCreate) bool {
	if len(b.Config.AllowedUsers) == 0 {
		return true
	}
	userID := ""
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}
	for _, id := range b.Config.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	user := i.Member.User
	if user == nil {
		user = i.User
	}
	log.Printf("[BOT] Command /%s by %s", i.ApplicationCommandData().Name, user.Username)

	if !b.checkPermission(i) {
		log.Printf("[BOT WARN] Unauthorized access attempt by %s", user.Username)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "⛔ Access Denied.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	switch i.ApplicationCommandData().Name {
	case "help":
		b.handleHelp(s, i)
	case "ping":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Pong! 🏓"},
		})
	case "list":
		b.handleList(s, i)
	case "upload":
		b.handleUpload(s, i)
	case "delete":
		b.handleDelete(s, i)
	}
}

func (b *Bot) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "Discord Vault 🛡️",
		Description: "High-security file storage using Discord and AES-256.",
		Color:       0x3b82f6,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "/upload", Value: "Store a file securely (max 25MB via Bot)"},
			{Name: "/list", Value: "List all secured assets"},
			{Name: "/delete [id]", Value: "Purge an asset from the vault"},
		},
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

func (b *Bot) handleUpload(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	attachment := i.ApplicationCommandData().Resolved.Attachments[options[0].Value.(string)]

	log.Printf("[BOT] Processing upload from Discord: %s", attachment.Filename)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "⏳ Processing & Encrypting..."},
	})

	resp, err := http.Get(attachment.URL)
	if err != nil {
		log.Printf("[BOT ERR] Failed to fetch attachment: %v", err)
		b.followup(i, "❌ Failed to fetch file.")
		return
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	encrypted, err := crypto.Encrypt(data, b.Config.EncryptionKey)
	if err != nil {
		log.Printf("[BOT ERR] Encryption failed: %v", err)
		b.followup(i, "❌ Encryption failed.")
		return
	}

	log.Printf("[BOT] Saving encrypted payload to storage channel...")
	msg, err := b.Session.ChannelFileSend(b.Config.ChannelID, fmt.Sprintf("%x.vault", sha256.Sum256(encrypted)), bytes.NewReader(encrypted))
	if err != nil {
		log.Printf("[BOT ERR] Discord storage failed: %v", err)
		b.followup(i, "❌ Could not save to storage channel.")
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	fileID, err := b.DB.SaveFile(attachment.Filename, int64(attachment.Size), hashStr)
	if err != nil {
		log.Printf("[BOT ERR] DB Save failed: %v", err)
		b.followup(i, "❌ Database error.")
		return
	}

	b.DB.SaveChunk(fileID, msg.ID, 1)
	log.Printf("[BOT] Success! Saved %s (ID: %d)", attachment.Filename, fileID)

	// Send notification log like web upload
	go b.NotifyUpload(attachment.Filename, int64(attachment.Size), 1, "Bot")

	b.followup(i, fmt.Sprintf("✅ Object secured. ID: **#%d**", fileID))
}

func (b *Bot) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	files, _ := b.DB.ListFiles()
	var sb strings.Builder
	sb.WriteString("📂 **Vault Assets:**\n\n")
	if len(files) == 0 {
		sb.WriteString("*Empty*")
	}
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("`#%d` **%s** (%s)\n", f.ID, f.Name, formatBytes(f.Size)))
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: sb.String()},
	})
}

func (b *Bot) handleDelete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	id := int(i.ApplicationCommandData().Options[0].IntValue())
	log.Printf("[BOT] Manual purge requested for ID: %d", id)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "💣 Purging..."},
	})

	chunks, _ := b.DB.GetChunks(id)
	for _, c := range chunks {
		s.ChannelMessageDelete(b.Config.ChannelID, c.MessageID)
	}

	b.DB.DeleteFile(id)
	log.Printf("[BOT] ID %d purged.", id)
	b.followup(i, "🧹 Purge complete.")
}

func (b *Bot) followup(i *discordgo.InteractionCreate, content string) {
	b.Session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
