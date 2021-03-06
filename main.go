package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/fzzy/radix/redis"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultExpires  = 86400
	ExitCodeError   = 111
	UnlockLUAScript = "if redis.call(\"get\",KEYS[1]) == ARGV[1]\nthen\nreturn redis.call(\"del\",KEYS[1])\nelse\nreturn 0\nend\n"
	Version         = "0.0.1"
	RetryInterval   = time.Duration(500) * time.Millisecond
)

var TrapSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGQUIT}

type Options struct {
	Redis    string
	Expires  int
	Keep     bool
	Wait     bool
	ExitCode int
}

func main() {
	code := run()
	os.Exit(code)
}

func parseOptions() (opt *Options, key string, program string, args []string) {
	var redis string
	var expires int
	var keep bool
	var noDelay bool
	var delay bool
	var exitZero bool
	var exitNonZero bool
	var showVersion bool

	flag.Usage = usage
	flag.StringVar(&redis, "redis", "127.0.0.1:6379", "redis-server host:port")
	flag.IntVar(&expires, "expires", DefaultExpires, "The lock will be auto-released after the expire time is reached.")
	flag.BoolVar(&keep, "keep", false, "Keep the lock after invoked command exited.")
	flag.BoolVar(&noDelay, "n", false, "No delay. If KEY is locked by another process, go-redis-setlock gives up.")
	flag.BoolVar(&delay, "N", true, "(Default.) Delay. If KEY is locked by another process, go-redis-setlock waits until it can obtain a new lock.")
	flag.BoolVar(&exitZero, "x", false, "If KEY is locked, go-redis-setlock exits zero.")
	flag.BoolVar(&exitNonZero, "X", true, "(Default.) If KEY is locked, go-redis-setlock prints an error message and exits nonzero.")
	flag.BoolVar(&showVersion, "version", false, fmt.Sprintf("version %s", Version))
	flag.Parse()

	if showVersion {
		fmt.Fprintf(os.Stderr, "version: %s\n", Version)
		os.Exit(0)
	}

	opt = &Options{
		Redis:    redis,
		Keep:     keep,
		Wait:     true,
		ExitCode: ExitCodeError,
		Expires:  expires,
	}
	if noDelay {
		opt.Wait = false
	}
	if exitZero {
		opt.ExitCode = 0
	}

	remainArgs := flag.Args()
	if len(remainArgs) >= 2 {
		key = remainArgs[0]
		program = remainArgs[1]
		if len(remainArgs) >= 3 {
			args = remainArgs[2:]
		}
	} else {
		usage()
	}

	return opt, key, program, args
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\n    go-redis-setlock [-nNxX] KEY program [ arg ... ]\n\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func run() int {
	opt, key, program, args := parseOptions()
	c, err := connectToRedisServer(opt)
	if err != nil {
		log.Printf("Redis server seems down: %s\n", err)
		return ExitCodeError
	}
	defer c.Close()

	if !validateRedisVersion(c) {
		return ExitCodeError
	}
	token, err := tryGetLock(c, opt, key)
	if err == nil {
		defer releaseLock(c, opt, key, token)
		code := invokeCommand(program, args)
		return code
	} else {
		log.Println(err)
		return opt.ExitCode
	}
}

func connectToRedisServer(opt *Options) (c *redis.Client, err error) {
	timeout := 0
	if opt.Wait {
		timeout = opt.Expires
	}
	start := time.Now()
	for {
		c, err = redis.DialTimeout("tcp", opt.Redis, time.Duration(timeout)*time.Second)
		if err == nil {
			break
		}
		end := time.Now()
		elapsed := int(end.Sub(start) / time.Millisecond) // msec
		if elapsed >= timeout*1000 {
			break
		}
		time.Sleep(RetryInterval)
	}
	return c, err
}

func validateRedisVersion(c *redis.Client) bool {
	version := ""

	r := c.Cmd("info")
	info, _ := r.Str()
	for _, line := range strings.Split(info, "\n") {
		pair := strings.SplitN(line, ":", 2)
		if pair[0] == "redis_version" {
			version = pair[1]
			break
		}
	}
	if version == "" {
		log.Printf("could not detect Redis server version from INFO outout. %s", info)
		return false
	}

	vNumbers := strings.SplitN(version, ".", 3)
	major, _ := strconv.Atoi(vNumbers[0])
	minor, _ := strconv.Atoi(vNumbers[1])
	rev, _ := strconv.Atoi(vNumbers[2])
	if (major >= 3) || (major == 2 && minor >= 7) || (major == 2 && minor == 6 && rev >= 12) {
		return true
	}
	log.Printf("required Redis server version >= 2.6.12. current server version is %s\n", version)
	return false
}

func tryGetLock(c *redis.Client, opt *Options, key string) (token string, err error) {
	token = createToken()
	gotLock := false
	for {
		r := c.Cmd("SET", key, token, "EX", opt.Expires, "NX")
		locked, _ := r.Str()
		if locked != "" {
			gotLock = true
			break
		} else if !opt.Wait {
			break
		} else {
			time.Sleep(RetryInterval)
		}
	}
	if gotLock {
		return token, nil
	} else {
		return "", errors.New("unable to lock")
	}
}

func releaseLock(c *redis.Client, opt *Options, key string, token string) (err error) {
	if opt.Keep {
		return nil
	} else {
		r := c.Cmd("EVAL", UnlockLUAScript, 1, key, token)
		return r.Err
	}
}

func invokeCommand(program string, args []string) (code int) {
	cmd := exec.Command(program, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Println(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Println(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Println(err)
	}
	err = cmd.Start()
	if err != nil {
		log.Println(err)
	}
	go func() {
		_, err := io.Copy(stdin, os.Stdin)
		if err == nil {
			stdin.Close()
		} else {
			log.Println(err)
			stdin.Close()
		}
	}()
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	var cmdErr error
	cmdCh := make(chan error)
	go func() {
		cmdCh <- cmd.Wait()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, TrapSignals...)
	select {
	case s := <-signalCh:
		cmd.Process.Signal(s) // forward to child
		switch sig := s.(type) {
		case syscall.Signal:
			code = int(sig)
			log.Printf("Got signal: %s(%d)", sig, sig)
		default:
			code = -1
		}
		<-cmdCh
	case cmdErr = <-cmdCh:
	}

	// http://qiita.com/hnakamur/items/5e6f22bda8334e190f63
	if cmdErr != nil {
		if e2, ok := cmdErr.(*exec.ExitError); ok {
			if s, ok := e2.Sys().(syscall.WaitStatus); ok {
				code = s.ExitStatus()
			} else {
				log.Println("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus.")
				return ExitCodeError
			}
		}
	}
	return code
}

func createToken() string {
	b := make([]byte, 16)
	crand.Read(b)
	return hex.EncodeToString(b)
}
