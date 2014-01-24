package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"github.com/fzzy/radix/redis"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultExpires                  = 86400
	ExitCodeRedisDead               = 1
	ExitCodeRedisUnsupportedVersion = 2
	ExitCodeCannotGetLock           = 3
	UnlockLUAScript                 = "if redis.call(\"get\",KEYS[1]) == ARGV[1]\nthen\nreturn redis.call(\"del\",KEYS[1])\nelse\nreturn 0\nend\n"
)

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

	flag.StringVar(&redis, "redis", "127.0.0.1:6379", "redis-server host:port")
	flag.IntVar(&expires, "expires", DefaultExpires, "The lock will be auto-released after the expire time is reached.")
	flag.BoolVar(&keep, "keep", false, "Keep the lock after invoked command exited.")
	flag.BoolVar(&noDelay, "n", false, "No delay. If KEY is locked by another process, redis-setlock gives up.")
	flag.BoolVar(&delay, "N", true, "(Default.) Delay. If KEY is locked by another process, redis-setlock waits until it can obtain a new lock.")
	flag.BoolVar(&exitZero, "x", false, "If KEY is locked, redis-setlock exits zero.")
	flag.BoolVar(&exitNonZero, "X", true, "(Default.) If KEY is locked, redis-setlock prints an error message and exits nonzero.")
	flag.Parse()

	opt = &Options{
		Redis:    redis,
		Keep:     keep,
		Wait:     true,
		ExitCode: ExitCodeCannotGetLock,
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
	}

	return opt, key, program, args
}

func run() int {
	opt, key, program, args := parseOptions()
	c, err := connectToRedisServer(opt)
	if err != nil {
		log.Printf("Redis server seems down: %s\n", err)
		return ExitCodeRedisDead
	}
	defer c.Close()

	if !validateRedisVersion(c) {
		return ExitCodeRedisUnsupportedVersion
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
		if elapsed >= timeout * 1000 {
			break
		}
		time.Sleep(time.Duration(500) * time.Millisecond)
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
			sleepMSec := rand.Intn(1000)
			time.Sleep(time.Duration(sleepMSec) * time.Millisecond)
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
		}
	}()
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmdErr := cmd.Wait()

	// http://qiita.com/hnakamur/items/5e6f22bda8334e190f63
	if cmdErr != nil {
		if e2, ok := cmdErr.(*exec.ExitError); ok {
			if s, ok := e2.Sys().(syscall.WaitStatus); ok {
				code = s.ExitStatus()
			} else {
				panic(errors.New("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus."))
			}
		}
	} else {
		code = 0
	}
	return code
}

func createToken() string {
	b := make([]byte, 16)
	_, err := crand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
