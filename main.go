package main

import (
	"errors"
	"flag"
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"io"
	"os"
	"os/exec"
	"strings"
)

var (
	cronConfig string
	missions   []*cronMission
)

type cronMission struct {
	Cron string
	Cmd  string
	Args []string
}

func init() {
	flag.StringVar(&cronConfig, "c", "cron", "cron config file")
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[%time%][%lvl%]: %msg% \n",
	})
	log.SetLevel(log.InfoLevel)
	if !pathExists("logs") {
		if err := os.MkdirAll("logs", 0o755); err != nil {
			log.Fatalf("failed create log folder")
		}
		log.Info("created log folder")
	}
	r, _ := rotatelogs.New("./logs/cron_log.%Y-%m-%d.log")
	mw := io.MultiWriter(os.Stdout, r)
	log.SetOutput(mw)
}

func main() {
	flag.Parse()
	file, err := os.ReadFile(cronConfig)
	if err != nil {
		log.Fatal("cannot read config file")
	}

	cfg := strings.ReplaceAll(string(file), "\r\n", "\n")
	lines := strings.Split(cfg, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}
		spl := strings.Split(line, " ")
		if len(spl) < 6 {
			log.Fatalf("invalid cron expression: %s", line)
		}
		cronExp := strings.Join(spl[0:6], " ")
		cmd := spl[6]
		args := spl[7:]
		missions = append(missions, &cronMission{Cron: cronExp, Cmd: cmd, Args: args})
	}

	specParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for pos, m := range missions {
		_, err := specParser.Parse(m.Cron)
		if err != nil {
			log.Fatalf("invalid cron expression: %s | pos %d", m.Cron, pos)
		}
	}

	c := cron.New(cron.WithSeconds())
	for _, m := range missions {
		// 需要copy一遍
		cronExp := m.Cron
		command := m.Cmd
		cmdArgs := m.Args
		fullCmd := fmt.Sprintf("%s %s", m.Cmd, strings.Join(m.Args, " "))
		_, err := c.AddFunc(cronExp, func() {
			log.Infof("run: %s %s", cronExp, fullCmd)
			cmd := exec.Command(command, cmdArgs...)
			err := cmd.Run()
			if err != nil {
				log.Errorf("%s %s exec failed: %s", cronExp, fullCmd, err)
			}
		})
		if err != nil {
			log.Fatalf("cron: %s %s add failed", cronExp, fullCmd)
		}
		log.Infof("added mission: %s %s", cronExp, fullCmd)
	}
	c.Start()
	select {}
}

// pathExists 判断给定path是否存在
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || errors.Is(err, os.ErrExist)
}
