package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

type WebhookContent struct {
	Content string  `json:"content"`
	Embeds  []Embed `json:"embeds"`
}

type Embed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

const (
	ColorYellow = "16311836"
	ColorBlue   = "43519"
	ColorGreen  = "12118406"
)

var (
	regexChatMessage    string = `^\[.*\]: \<(.*)\> (.*)`
	regexJoin           string = `^\[.*\]: (.*) joined the game`
	regexLeft           string = `^\[.*\]: (.*) left the game`
	regexServerStarting string = `^\[.*\]: Starting minecraft server`
	regexServerStarted  string = `^\[.*\]: Done \(.*\)! For help, type "help"`
	regexServerStop     string = `^\[.*\]: Stopping server`
	regexAdvancement    string = `^\[.*\]: (.*) has made the advancement \[(.*)\]`
)

func main() {
	if len(os.Args) < 2 {
		slog.Error("missing log file")
		slog.Error("usage: <path/to/logfile> <webhookUrl>")
		return
	}

	if len(os.Args) < 3 {
		slog.Error("missing webook url")
		slog.Error("usage: <path/to/logfile> <webhookUrl>")
		return
	}

	filePath := os.Args[1]
	webhookUrl := os.Args[2]
	slog.Info("start reading...", "filePath", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("can't read log file", "filePath", filePath)
		return
	}
	defer file.Close()

	slog.Info("start")

	fileInfo, err := file.Stat()
	if err != nil {
		slog.Error("error getting file info", "error", err)
		return
	}

	logChannels := make(chan string, 2)
	go processMessageQueue(webhookUrl, logChannels)

	// Read from last line
	fileSize := fileInfo.Size()
	file.Seek(fileSize, io.SeekStart)
	slog.Debug("file info", "fileSize", fileSize)
	reader := bufio.NewReader(file)

	slog.Debug("start loop")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(500 * time.Millisecond)

				truncated, err := isTruncated(file)
				if err != nil {
					break
				}

				if truncated {
					_, err := file.Seek(0, io.SeekStart)
					if err != nil {
						break
					}
				}
				continue
			} else {
				slog.Error("unknown error cannot read file", "error", err)
			}

			break
		}

		if !isChatMessage(line) {
			// NOTE: Add message to channel in goroutine so it is not blocking reading log
			go func() {
				logChannels <- line
			}()
		}
		slog.Info("log", "message", string(line))
	}
	slog.Info("stopped")
}

func isTruncated(file *os.File) (bool, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}

	return currentPos > fileInfo.Size(), nil
}

func parseJoinMessage(message string) string {
	reg := regexp.MustCompile(regexJoin)
	matches := reg.FindStringSubmatch(message)

	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func parseLeftMessage(message string) string {
	reg := regexp.MustCompile(regexLeft)
	matches := reg.FindStringSubmatch(message)

	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func parseAdavancementMessage(message string) (string, string) {
	reg := regexp.MustCompile(regexAdvancement)
	matches := reg.FindStringSubmatch(message)

	if len(matches) > 1 {
		return matches[1], matches[2]
	}

	return "", ""
}

func isChatMessage(msg string) bool {
	reg := regexp.MustCompile(regexChatMessage)
	return reg.MatchString(msg)
}

func parseChatMessage(msg string) []string {
	reg := regexp.MustCompile(regexChatMessage)
	matches := reg.FindStringSubmatch(msg)

	if len(matches) == 0 {
		return []string{}
	}

	return []string{
		matches[1],
		matches[2],
	}
}

func processMessageQueue(webhookUrl string, channel chan string) {
	slog.Debug("processMessageQueue start")
	playerJoin := map[string]time.Time{}

	for {
		message := <-channel
		slog.Debug("processMessageQueue recieve message", "message", message)

		if regexp.MustCompile(regexServerStarting).MatchString(message) {
			postWebhook(webhookUrl, Embed{
				Title:       "Server starting... Please wait",
				Description: "",
				Color:       ColorBlue,
			})
		} else if regexp.MustCompile(regexServerStarted).MatchString(message) {
			postWebhook(webhookUrl, Embed{
				Title:       "Server started! Ready to join!",
				Description: "",
				Color:       ColorBlue,
			})
		} else if regexp.MustCompile(regexServerStop).MatchString(message) {
			postWebhook(webhookUrl, Embed{
				Title:       "Server stopping...",
				Description: "",
				Color:       ColorBlue,
			})
		} else if regexp.MustCompile(regexJoin).MatchString(message) {
			playerName := parseJoinMessage(message)
			playerJoin[playerName] = time.Now()
			postWebhook(webhookUrl, Embed{
				Title:       fmt.Sprintf("%s joined the game", playerName),
				Description: "",
				Color:       ColorYellow,
			})
		} else if regexp.MustCompile(regexLeft).MatchString(message) {
			playerName := parseLeftMessage(message)
			postWebhook(webhookUrl, Embed{
				Title:       fmt.Sprintf("%s left the game", playerName),
				Description: "",
				Color:       ColorYellow,
			})
		} else if regexp.MustCompile(regexAdvancement).MatchString(message) {
			playerName, advancement := parseAdavancementMessage(message)
			postWebhook(webhookUrl, Embed{
				Title:       fmt.Sprintf("%s has made the advancement", playerName),
				Description: advancement,
				Color:       ColorGreen,
			})
		}
	}
}

func postWebhook(url string, content Embed) {
	values := map[string]interface{}{"content": "", "embeds": []Embed{content}}
	jsonData, err := json.Marshal(values)
	if err != nil {
		log.Fatal(err)
	}

	for {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			slog.Error("error can't post to webhook")
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			waitTime := 1
			if resp.Header.Get("X-RateLimit-Reset") != "" {
				if rateLimit, err := strconv.ParseFloat(resp.Header.Get("X-RateLimit-Reset"), 32); err == nil {
					waitTime = int(rateLimit)
				}
			}
			time.Sleep(time.Duration(waitTime) * time.Second)

			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("can't read response body")
		}

		slog.Info("post webhook", "message", string(body))
		break
	}
}
