// Package main provides a Discord bot that reads msgs from discord channels and stores them in PebbleDB
// which are then sent to a specified Discord channel. It includes functionality to handle
// message content and attachments, and ensures proper error handling and resource cleanup.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/go-logr/zapr"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type attachment struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

type message struct {
	ID          string       `json:"id"`
	ChannelID   string       `json:"channel_id"`
	Content     string       `json:"content"`
	Author      string       `json:"author"`
	Pinned      bool         `json:"pinned"`
	Timestamp   int64        `json:"timestamp"`
	Attachments []attachment `json:"attachments"`
}

func main() {
	logger, _ := zap.Config{
		Encoding:    "json",
		Level:       zap.NewAtomicLevelAt(zapcore.DebugLevel),
		OutputPaths: []string{"stdout"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:   "message",
			LevelKey:     "level",
			EncodeLevel:  zapcore.CapitalLevelEncoder,
			TimeKey:      "time",
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		},
	}.Build()

	log := zapr.NewLogger(logger)
	defer logger.Sync() // nolint: errcheck

	conf := struct {
		from  cli.StringSlice
		to    string
		token string
	}{}

	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "from",
				EnvVars:     []string{"FROM"},
				Destination: &conf.from,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "to",
				EnvVars:     []string{"TO"},
				Destination: &conf.to,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "token",
				EnvVars:     []string{"TOKEN"},
				Destination: &conf.token,
				Required:    true,
			},
		},
		Action: func(c *cli.Context) error {
			opt := pebbleDBOpt()

			db, err := pebble.Open("msgs.db", opt)
			if err != nil {
				return err
			}
			defer func() {
				os.RemoveAll("msgs.db") // nolint: errcheck
			}()
			defer db.Close() // nolint: errcheck

			discord, err := discordgo.New(fmt.Sprintf("Bot %s", conf.token))
			if err != nil {
				return fmt.Errorf("discordgo.New: %w", err)
			}

			if err := getAndStoreMsgs(db, discord, conf.from.Value()); err != nil {
				return fmt.Errorf("getAndStoreMsgs: %w", err)
			}

			if err := readStoredMsgsAndSend(c.Context, db, discord, conf.to); err != nil {
				return fmt.Errorf("readStoredMsgs: %w", err)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err, "run")
		os.Exit(1)
	}
}

func getAndStoreMsgs(db *pebble.DB, discord *discordgo.Session, fromChannelIDs []string) error {
	for _, from := range fromChannelIDs {
		beforeID := ""
		for {
			msgs, err := discord.ChannelMessages(from, 100, beforeID, "", "")
			if err != nil {
				return fmt.Errorf("discord.ChannelMessages: %w", err)
			}
			if len(msgs) == 0 {
				beforeID = ""
				break
			}

			last := len(msgs) - 1
			beforeID = msgs[last].ID

			if err := storeMsgs(db, msgs); err != nil {
				return fmt.Errorf("storeAllMsgs: %w", err)
			}
		}
	}
	return nil
}

func storeMsgs(db *pebble.DB, msgs []*discordgo.Message) error {
	batch := db.NewBatch()
	defer batch.Close() // nolint: errcheck

	for _, msg := range msgs {
		att := make([]attachment, len(msg.Attachments))
		for i, a := range msg.Attachments {
			att[i] = attachment{
				Filename: a.Filename,
				URL:      a.URL,
			}
		}

		m := message{
			ID:          msg.ID,
			ChannelID:   msg.ChannelID,
			Author:      msg.Author.Username,
			Content:     msg.Content,
			Pinned:      msg.Pinned,
			Timestamp:   msg.Timestamp.UnixMicro(),
			Attachments: att,
		}

		b, err := json.Marshal(m)
		if err != nil {
			return err
		}

		if err = batch.Set(key(fmt.Sprintf("%d", m.Timestamp), m.ID), b, pebble.Sync); err != nil {
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}

func key(keys ...string) []byte {
	return []byte(strings.Join(keys, "_"))
}

func downloadFile(url string) (*os.File, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	tmpFile, err := os.CreateTemp("", "attachment-*")
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close() // nolint: errcheck
		return nil, err
	}

	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		tmpFile.Close() // nolint: errcheck
		return nil, err
	}

	return tmpFile, nil
}

func pebbleDBOpt() *pebble.Options {
	opt := &pebble.Options{
		MaxOpenFiles:                16,
		MemTableSize:                1<<30 - 1, // Max 1 GB
		MemTableStopWritesThreshold: 2,
		// MaxConcurrentCompactions: func() int { return runtime.NumCPU() },
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		},
	}
	opt.Experimental.ReadSamplingMultiplier = -1

	return opt
}

func readStoredMsgsAndSend(ctx context.Context, db *pebble.DB, discord *discordgo.Session, toChannelID string) error {
	it, err := db.NewIterWithContext(ctx, &pebble.IterOptions{})
	if err != nil {
		return fmt.Errorf("db.NewIterWithContext: %w", err)
	}
	defer it.Close() // nolint: errcheck

	it.First()
	for ; it.Valid(); it.Next() {
		var d message
		err := json.Unmarshal(it.Value(), &d)
		if err != nil {
			return fmt.Errorf("json.Unmarshal: %w", err)
		}

		if err := sendStoredMsg(toChannelID, d, discord); err != nil {
			return fmt.Errorf("sendStoredMsg: %w", err)
		}
	}

	return nil
}

func sendStoredMsg(to string, msg message, discord *discordgo.Session) error {
	nmsg, err := discord.ChannelMessageSend(to, fmt.Sprintf("%s: %s", msg.Author, msg.Content))
	if err != nil {
		return fmt.Errorf("discord.ChannelMessageSend: %w", err)
	}
	if msg.Pinned {
		err = discord.ChannelMessagePin(to, nmsg.ID)
		if err != nil {
			return fmt.Errorf("discord.ChannelMessagePin: %w", err)
		}
	}
	for _, attachment := range msg.Attachments {
		file, err := downloadFile(attachment.URL)
		if err != nil {
			return fmt.Errorf("downloadFile: %w", err)
		}

		_, err = discord.ChannelFileSend(to, attachment.Filename, file)
		if err != nil {
			return fmt.Errorf("discord.ChannelFileSend: %w", err)
		}
		os.Remove(file.Name()) // nolint: errcheck
	}

	return nil
}
