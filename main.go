package main

import (
	"bufio"
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
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	cronConfig     string
	redirectOutput bool
	missions       []*cronMission
	sysType        = runtime.GOOS
	executablePath string
)

type cronMission struct {
	Cron string
	Cmd  string
	Args []string
}

func init() {
	flag.StringVar(&cronConfig, "c", "cron", "cron config file")
	flag.BoolVar(&redirectOutput, "o", false, "redirect command stdout/stderr to log")
	ls := "\n"
	if sysType == "windows" {
		ls = "\r\n"
	}
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[%time%][%lvl%]: %msg% " + ls,
	})
	log.SetLevel(log.InfoLevel)
	executablePath, _ = os.Executable()
	logPath := path.Join(filepath.Dir(executablePath), "logs")
	if !pathExists(logPath) {
		if err := os.MkdirAll(logPath, 0o755); err != nil {
			log.Fatalf("failed create log folder")
		}
		log.Info("created log folder")
	}
	r, _ := rotatelogs.New(path.Join(filepath.Dir(executablePath), "logs", "cron_log.%Y-%m-%d.log"))
	mw := io.MultiWriter(os.Stdout, r)
	log.SetOutput(mw)
}

func main() {
	flag.Parse()
	file, err := os.ReadFile(path.Join(filepath.Dir(executablePath), cronConfig))
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
			if redirectOutput {
				stdout, _ := cmd.StdoutPipe()
				stderr, _ := cmd.StderrPipe()
				multi := io.MultiReader(stdout, stderr)
				if err := cmd.Start(); err != nil {
					log.Errorf("%s %s exec failed: %s", cronExp, fullCmd, err)
				}
				in := bufio.NewScanner(multi)
				for in.Scan() {
					log.Infof(in.Text())
				}
				if err := in.Err(); err != nil {
					log.Errorf("error: %s", err)
				}
			} else {
				err := cmd.Run()
				if err != nil {
					log.Errorf("%s %s exec failed: %s", cronExp, fullCmd, err)
				}
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
