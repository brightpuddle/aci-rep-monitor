package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/alexflint/go-arg"
	"golang.org/x/term"
)

type args struct {
	APIC        string `arg:"-a" help:"APIC host or IP"`
	Usr         string `arg:"-u" help:"Username"`
	Pwd         string `arg:"-p" help:"Password"`
	HTTPTimeout int    `arg:"--http-timeout" default:"180" help:"HTTP timeout"`
	ClearDelay  int    `arg:"--clear-delay" default:"30" help:"Delay to clear EP"`
}

func (args) Description() string {
	return "Rogue EP Detection monitoring tool"
}

func (args) Version() string {
	if version == "" {
		return "development build"
	}
	return fmt.Sprintf("verions %s", version)
}

func getInput(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s ", prompt)
	input, _ := reader.ReadString('\n')
	return strings.Trim(input, "\r\n")
}

func getPassword(prompt string) string {
	fmt.Print(prompt + " ")
	pwd, _ := term.ReadPassword(int(syscall.Stdin))
	return string(pwd)
}

func newArgs() args {
	a := args{}
	arg.MustParse(&a)
	if a.APIC == "" {
		a.APIC = getInput("APIC host or IP:")
	}
	if a.Usr == "" {
		a.Usr = getInput("Username:")
	}
	if a.Pwd == "" {
		a.Pwd = getPassword("Password:")
	}
	return a
}
