package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase          = "https://discord.com/api/v10"
	defaultGuild     = "EgaoShark Waifu Culture"
	defaultChannel   = "nsfw_perfect"
	defaultOutputDir = "/home/kyz/Pictures/EgaoSharkWaifuCulture"
	planPath         = ".disbot-plan.json"
	appName          = "EgaoShark Media Archiver"
	appDescription   = "Archives image attachments from authorized Discord channels."
	botPermissions   = "66560" // VIEW_CHANNEL | READ_MESSAGE_HISTORY
)

type attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type message struct {
	ID           string        `json:"id"`
	Content      string        `json:"content"`
	Attachments  []attachment  `json:"attachments"`
	Embeds       []embed       `json:"embeds"`
	StickerItems []stickerItem `json:"sticker_items"`
}

type embed struct {
	Image     *embedMedia `json:"image"`
	Thumbnail *embedMedia `json:"thumbnail"`
}

type embedMedia struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url"`
}

type stickerItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FormatType int    `json:"format_type"`
}

type guild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type channel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	NSFW    bool   `json:"nsfw"`
	GuildID string `json:"guild_id"`
}

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
	Avatar   string `json:"avatar"`
}

type application struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Flags       int    `json:"flags"`
}

type planItem struct {
	MessageID     string   `json:"message_id"`
	AttachmentID  string   `json:"attachment_id"`
	Filename      string   `json:"filename"`
	URL           string   `json:"url"`
	AlternateURLs []string `json:"alternate_urls,omitempty"`
	ContentType   string   `json:"content_type,omitempty"`
	Size          int64    `json:"size"`
}

type downloadPlan struct {
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	GuildID     string     `json:"guild_id"`
	GuildName   string     `json:"guild_name"`
	ChannelID   string     `json:"channel_id"`
	ChannelName string     `json:"channel_name"`
	OutputDir   string     `json:"output_dir"`
	Items       []planItem `json:"items"`
}

type discordClient struct {
	token string
	http  *http.Client
}

func main() {
	if err := loadDotEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}
	token := firstNonempty(os.Getenv("DISCORD_BOT_TOKEN"), os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		fatal(errors.New("DISCORD_BOT_TOKEN (or legacy DISCORD_TOKEN) is not set"))
	}
	client := &discordClient{token: token, http: &http.Client{Timeout: 90 * time.Second}}
	ctx := context.Background()

	command := ""
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "info":
		err = runInfo(ctx, client)
	case "configure":
		err = runConfigure(ctx, client)
	case "enable-message-content":
		err = runEnableMessageContent(ctx, client)
	case "invite":
		err = runInvite(ctx, client)
	case "plan":
		err = runPlan(ctx, client)
	case "download":
		err = runDownload(ctx, client)
	default:
		err = fmt.Errorf("usage: disbot {info|configure|enable-message-content|invite|plan|download}")
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *discordClient) do(ctx context.Context, method, path string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 6; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bot "+c.token)
		req.Header.Set("User-Agent", "DiscordBot (https://github.com/kokizzu/disbot, 1.0)")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			var limited struct {
				RetryAfter float64 `json:"retry_after"`
			}
			_ = json.Unmarshal(responseBody, &limited)
			delay := time.Duration(limited.RetryAfter * float64(time.Second))
			if delay <= 0 {
				delay = time.Second
			}
			time.Sleep(delay)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Discord API %s %s returned %s: %s", method, path, resp.Status, compactAPIError(responseBody))
		}
		if out != nil && len(responseBody) > 0 {
			if err := json.Unmarshal(responseBody, out); err != nil {
				return fmt.Errorf("decode Discord response: %w", err)
			}
		}
		return nil
	}
	return errors.New("Discord API rate limit retry budget exhausted")
}

func compactAPIError(data []byte) string {
	var value struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if json.Unmarshal(data, &value) == nil && value.Message != "" {
		return fmt.Sprintf("%s (code %d)", value.Message, value.Code)
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return text
}

func runInfo(ctx context.Context, client *discordClient) error {
	var me user
	if err := client.do(ctx, http.MethodGet, "/users/@me", nil, &me); err != nil {
		return err
	}
	var app application
	if err := client.do(ctx, http.MethodGet, "/applications/@me", nil, &app); err != nil {
		return err
	}
	fmt.Printf("Bot: %s (%s), bot=%v\n", me.Username, me.ID, me.Bot)
	fmt.Printf("Application: %s (%s)\n", app.Name, app.ID)
	fmt.Printf("Description configured: %v\n", app.Description == appDescription)
	fmt.Printf("Application icon configured: %v\n", app.Icon != "")
	fmt.Printf("Bot avatar configured: %v\n", me.Avatar != "")
	fmt.Printf("Message Content Intent enabled: %v (flags=%d)\n", messageContentEnabled(app.Flags), app.Flags)
	fmt.Printf("Install URL: https://discord.com/oauth2/authorize?client_id=%s&permissions=%s&integration_type=0&scope=bot\n", app.ID, botPermissions)
	return nil
}

func messageContentEnabled(flags int) bool {
	const (
		gatewayMessageContent        = 1 << 18
		gatewayMessageContentLimited = 1 << 19
	)
	return flags&(gatewayMessageContent|gatewayMessageContentLimited) != 0
}

func withMessageContentEnabled(flags int) int {
	const gatewayMessageContentLimited = 1 << 19
	return flags | gatewayMessageContentLimited
}

func runEnableMessageContent(ctx context.Context, client *discordClient) error {
	var app application
	if err := client.do(ctx, http.MethodGet, "/applications/@me", nil, &app); err != nil {
		return err
	}
	if messageContentEnabled(app.Flags) {
		fmt.Printf("Message Content Intent is already enabled (flags=%d).\n", app.Flags)
		return nil
	}
	requestedFlags := withMessageContentEnabled(app.Flags)
	if err := client.do(ctx, http.MethodPatch, "/applications/@me", map[string]any{"flags": requestedFlags}, &app); err != nil {
		return fmt.Errorf("enable Message Content Intent: %w", err)
	}
	if !messageContentEnabled(app.Flags) {
		return fmt.Errorf("Discord accepted the update but did not enable Message Content Intent (flags=%d)", app.Flags)
	}
	fmt.Printf("Enabled Message Content Intent (flags=%d).\n", app.Flags)
	return nil
}

func runInvite(ctx context.Context, client *discordClient) error {
	var app application
	if err := client.do(ctx, http.MethodGet, "/applications/@me", nil, &app); err != nil {
		return err
	}
	fmt.Printf("https://discord.com/oauth2/authorize?client_id=%s&permissions=%s&integration_type=0&scope=bot\n", app.ID, botPermissions)
	return nil
}

func runConfigure(ctx context.Context, client *discordClient) error {
	iconData, err := os.ReadFile("assets/icon.png")
	if err != nil {
		return fmt.Errorf("read icon: %w", err)
	}
	encodedIcon := "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconData)
	var app application
	appPayload := map[string]any{
		"description": appDescription,
		"icon":        encodedIcon,
		"tags":        []string{"media-archiver", "image-backup"},
		"install_params": map[string]any{
			"scopes":      []string{"bot"},
			"permissions": botPermissions,
		},
	}
	if err := client.do(ctx, http.MethodPatch, "/applications/@me", appPayload, &app); err != nil {
		return fmt.Errorf("configure application: %w", err)
	}
	var me user
	if err := client.do(ctx, http.MethodPatch, "/users/@me", map[string]any{
		"username": appName,
		"avatar":   encodedIcon,
	}, &me); err != nil {
		return fmt.Errorf("configure bot user: %w", err)
	}
	fmt.Printf("Configured application %q and bot user %q.\n", app.Name, me.Username)
	return nil
}

func runPlan(ctx context.Context, client *discordClient) error {
	guildName := firstNonempty(os.Getenv("DISCORD_GUILD_NAME"), defaultGuild)
	channelName := firstNonempty(os.Getenv("DISCORD_CHANNEL_NAME"), defaultChannel)
	outputDir := firstNonempty(os.Getenv("DOWNLOAD_DIR"), defaultOutputDir)
	targetGuild, targetChannel, err := resolveTarget(ctx, client, guildName, channelName)
	if err != nil {
		return err
	}
	items, messageCount, err := collectImages(ctx, client, targetChannel.ID)
	if err != nil {
		return err
	}
	plan := downloadPlan{
		Version: 1, CreatedAt: time.Now(), GuildID: targetGuild.ID, GuildName: targetGuild.Name,
		ChannelID: targetChannel.ID, ChannelName: targetChannel.Name, OutputDir: outputDir, Items: items,
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(planPath, append(data, '\n'), 0o600); err != nil {
		return err
	}
	var total int64
	for _, item := range items {
		total += item.Size
	}
	fmt.Printf("Guild: %s (%s)\n", targetGuild.Name, targetGuild.ID)
	fmt.Printf("Channel: %s (%s), age-restricted=%v\n", targetChannel.Name, targetChannel.ID, targetChannel.NSFW)
	fmt.Printf("Scanned messages: %d\n", messageCount)
	fmt.Printf("Planned images: %d (%s)\n", len(items), humanBytes(total))
	fmt.Printf("Destination: %s\n", outputDir)
	fmt.Printf("Checkpoint: %s\n", planPath)
	return nil
}

func resolveTarget(ctx context.Context, client *discordClient, guildName, channelName string) (guild, channel, error) {
	var guilds []guild
	if err := client.do(ctx, http.MethodGet, "/users/@me/guilds?limit=200", nil, &guilds); err != nil {
		return guild{}, channel{}, fmt.Errorf("list bot guilds: %w", err)
	}
	var selectedGuild *guild
	if id := strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")); id != "" {
		for index := range guilds {
			if guilds[index].ID == id {
				selectedGuild = &guilds[index]
				break
			}
		}
	} else {
		for index := range guilds {
			if strings.EqualFold(guilds[index].Name, guildName) {
				selectedGuild = &guilds[index]
				break
			}
		}
	}
	if selectedGuild == nil {
		return guild{}, channel{}, fmt.Errorf("bot is not installed in guild %q", guildName)
	}
	var channels []channel
	if err := client.do(ctx, http.MethodGet, "/guilds/"+selectedGuild.ID+"/channels", nil, &channels); err != nil {
		return guild{}, channel{}, fmt.Errorf("list guild channels: %w", err)
	}
	channelID := strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID"))
	for _, candidate := range channels {
		if (channelID != "" && candidate.ID == channelID) || (channelID == "" && strings.EqualFold(candidate.Name, channelName)) {
			return *selectedGuild, candidate, nil
		}
	}
	return guild{}, channel{}, fmt.Errorf("channel %q was not found or is not visible to the bot", channelName)
}

func collectImages(ctx context.Context, client *discordClient, channelID string) ([]planItem, int, error) {
	var items []planItem
	messageCount := 0
	before := ""
	for {
		path := "/channels/" + channelID + "/messages?limit=100"
		if before != "" {
			path += "&before=" + url.QueryEscape(before)
		}
		var messages []message
		if err := client.do(ctx, http.MethodGet, path, nil, &messages); err != nil {
			return nil, messageCount, fmt.Errorf("read channel messages: %w", err)
		}
		if len(messages) == 0 {
			break
		}
		messageCount += len(messages)
		for _, msg := range messages {
			items = append(items, imageItemsFromMessage(msg)...)
		}
		before = messages[len(messages)-1].ID
		if len(messages) < 100 {
			break
		}
	}
	return items, messageCount, nil
}

func runDownload(ctx context.Context, client *discordClient) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("read %s (run make plan first): %w", planPath, err)
	}
	var plan downloadPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("decode plan: %w", err)
	}
	if plan.Version != 1 || plan.OutputDir == "" {
		return errors.New("unsupported or incomplete download plan")
	}
	if err := os.MkdirAll(plan.OutputDir, 0o750); err != nil {
		return err
	}
	type downloadFailure struct {
		Index    int    `json:"index"`
		Filename string `json:"filename"`
		Error    string `json:"error"`
	}
	downloaded, skipped := 0, 0
	failures := make([]downloadFailure, 0)
	for index, item := range plan.Items {
		name := outputFilename(item.MessageID, attachment{ID: item.AttachmentID, Filename: item.Filename})
		destination := filepath.Join(plan.OutputDir, name)
		if info, err := os.Stat(destination); err == nil && (item.Size <= 0 || info.Size() == item.Size) {
			skipped++
			continue
		} else if err == nil {
			return fmt.Errorf("refusing to overwrite size-mismatched file: %s", destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := downloadOne(ctx, client.http, item, destination); err != nil {
			failures = append(failures, downloadFailure{Index: index + 1, Filename: item.Filename, Error: err.Error()})
			fmt.Printf("Unavailable item %d/%d: %s\n", index+1, len(plan.Items), item.Filename)
			continue
		}
		downloaded++
		if downloaded%25 == 0 {
			fmt.Printf("Downloaded %d/%d new files...\n", downloaded, len(plan.Items)-skipped)
		}
	}
	failureData, err := json.MarshalIndent(failures, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(".disbot-download-failures.json", append(failureData, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Printf("Complete: downloaded=%d, already-present=%d, unavailable=%d, destination=%s\n", downloaded, skipped, len(failures), plan.OutputDir)
	if len(failures) > 0 {
		fmt.Println("Failure checkpoint: .disbot-download-failures.json")
	}
	return nil
}

func downloadOne(ctx context.Context, httpClient *http.Client, item planItem, destination string) error {
	urls := append([]string{item.URL}, item.AlternateURLs...)
	errorsSeen := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		if rawURL == "" {
			continue
		}
		if err := downloadURL(ctx, httpClient, rawURL, item, destination); err == nil {
			return nil
		} else {
			errorsSeen = append(errorsSeen, err.Error())
		}
	}
	return fmt.Errorf("all %d source URLs failed: %s", len(errorsSeen), strings.Join(errorsSeen, "; "))
}

func downloadURL(ctx context.Context, httpClient *http.Client, rawURL string, item planItem, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("attachment server returned %s", resp.Status)
	}
	temporary := destination + ".part"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(temporary)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return closeErr
	}
	if item.Size > 0 && written != item.Size {
		_ = os.Remove(temporary)
		return fmt.Errorf("size mismatch for %s: got %d, want %d", item.Filename, written, item.Size)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func isImageAttachment(item attachment) bool {
	if strings.HasPrefix(strings.ToLower(item.ContentType), "image/") {
		return true
	}
	extension := strings.ToLower(filepath.Ext(item.Filename))
	if mediaType := mime.TypeByExtension(extension); strings.HasPrefix(mediaType, "image/") {
		return true
	}
	return isImageExtension(extension)
}

func isImageExtension(extension string) bool {
	extension = strings.ToLower(extension)
	switch extension {
	case ".avif", ".bmp", ".gif", ".heic", ".heif", ".jfif", ".jpeg", ".jpg", ".png", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
	}
}

func imageItemsFromMessage(msg message) []planItem {
	items := make([]planItem, 0, len(msg.Attachments)+len(msg.Embeds)*2)
	seen := make(map[string]bool)
	add := func(id, filename, rawURL, contentType string, size int64, alternateURLs ...string) {
		if rawURL == "" || seen[rawURL] {
			return
		}
		seen[rawURL] = true
		items = append(items, planItem{
			MessageID: msg.ID, AttachmentID: id, Filename: filename,
			URL: rawURL, AlternateURLs: uniqueAlternates(rawURL, alternateURLs...), ContentType: contentType, Size: size,
		})
	}

	for _, item := range msg.Attachments {
		if isImageAttachment(item) {
			add(item.ID, item.Filename, item.URL, item.ContentType, item.Size, item.ProxyURL)
		}
	}
	for index, item := range msg.Embeds {
		if item.Image != nil {
			rawURL := firstNonempty(item.Image.URL, item.Image.ProxyURL)
			add(fmt.Sprintf("embed-%d-image", index+1), filenameFromURL(rawURL, fmt.Sprintf("embed-%d-image", index+1)), rawURL, "", 0, item.Image.ProxyURL)
		}
		if item.Thumbnail != nil {
			rawURL := firstNonempty(item.Thumbnail.URL, item.Thumbnail.ProxyURL)
			add(fmt.Sprintf("embed-%d-thumbnail", index+1), filenameFromURL(rawURL, fmt.Sprintf("embed-%d-thumbnail", index+1)), rawURL, "", 0, item.Thumbnail.ProxyURL)
		}
	}
	linkIndex := 0
	for _, field := range strings.Fields(msg.Content) {
		rawURL := strings.Trim(field, "<>[](){}\"'.,")
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !isImageExtension(filepath.Ext(parsed.Path)) {
			continue
		}
		linkIndex++
		add(fmt.Sprintf("link-%d", linkIndex), filenameFromURL(rawURL, fmt.Sprintf("linked-image-%d", linkIndex)), rawURL, "", 0)
	}
	for _, item := range msg.StickerItems {
		extension := ".png"
		if item.FormatType == 4 {
			extension = ".gif"
		}
		if item.FormatType == 3 { // Lottie JSON is not a raster image.
			continue
		}
		rawURL := "https://media.discordapp.net/stickers/" + item.ID + extension
		alternateURL := "https://cdn.discordapp.com/stickers/" + item.ID + extension
		add("sticker-"+item.ID, safeFilename(item.Name)+extension, rawURL, "", 0, alternateURL)
	}
	return items
}

func uniqueAlternates(primary string, candidates ...string) []string {
	seen := map[string]bool{primary: true, "": true}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		result = append(result, candidate)
	}
	return result
}

func filenameFromURL(rawURL, fallback string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fallback
	}
	name := safeFilename(filepath.Base(parsed.Path))
	if name == "attachment" || name == "." || name == "/" {
		return fallback
	}
	return name
}

func safeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = filepath.Base(name)
	var builder strings.Builder
	for _, char := range name {
		if char < 32 || char == 127 {
			builder.WriteByte('_')
			continue
		}
		builder.WriteRune(char)
	}
	name = strings.TrimSpace(builder.String())
	if name == "" || name == "." {
		return "attachment"
	}
	return name
}

func outputFilename(messageID string, item attachment) string {
	return strings.Join([]string{messageID, item.ID, safeFilename(item.Filename)}, "_")
}

func humanBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}
	divisor, exponent := unit, 0
	for value := size / unit; value >= unit && exponent < 4; value /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(divisor), "KMGTPE"[exponent])
}
