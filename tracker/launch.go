package tracker

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/fearful-symmetry/garlic"
)

type TrackedProcess struct {
	Children []*TrackedProcess
	PID      uint32
	Start    time.Time
	End      time.Time
	Done     bool
	Name     string //TODO: I don't query for this information, but could be done
	Args     string
}

//to do this appropriately, we need to:
//1. buffer all incoming PCN calls.
//2. Start the process, and get it's PID
//3. Replay the buffer, to look for events
//4. Capture events as normal

//Launch will launch a process with the given arguments,
//and tracks process completion times.
func Launch(g *garlic.CnConn, name string, arg ...string) {
	ctx, _ := context.WithCancel(context.Background())

	events := make(chan []garlic.ProcEvent, 50)
	parentProcessChan := make(chan *TrackedProcess, 1)
	watchingCancel := make(chan bool, 1)
	loopCancel := make(chan bool, 1)
	go startWatching(g, events, watchingCancel)

	// launch the process
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// get the pid
	err := cmd.Start()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	tp := &TrackedProcess{
		Start: time.Now(),
		PID:   uint32(cmd.Process.Pid),
	}
	pidInfo, err := ps.FindProcess(cmd.Process.Pid)
	if err != nil {
		tp.Name = name
	} else {
		tp.Name = pidInfo.Executable()
	}

	//send the process to be tracked
	parentProcessChan <- tp

	//TODO: set a timeout and cancel if we're over the limit
	eventLoop(events, parentProcessChan, loopCancel)

	// once everything is done, or a timeout occurs,
	// print out the call stack/times
	print(tp)
}

func startWatching(gC *garlic.CnConn, events chan []garlic.ProcEvent, cancel chan bool) {
	for {
		select {
		case <-cancel:
			//just close the loop
			return
		default:
			data, err := gC.ReadPCN()
			if err != nil {
				log.Printf("Error reading from PCN: %s\n", err)
				continue
			}
			events <- data
		}
	}
}

func eventLoop(eventsChan chan []garlic.ProcEvent, parentProcessChan chan *TrackedProcess, cancelChan chan bool) {
	var err error
	var finished bool
	var parentProcess *TrackedProcess
	for {
		select {
		case <-cancelChan:
			//TODO: print out the current results? this will be incomplete,
			//and the printProcessTree expects an end time, so this is work to be done.
			return
		case parentProcess = <-parentProcessChan:
			//add this TrackedProcess to be watched by a sentry
			log.Printf("Parent process with PID %v being watched for.\n", parentProcess.PID)
			continue
		case newEvents := <-eventsChan:
			if parentProcess == nil {
				//TODO: we don't have a parent process yet, so cache the event for now. This is an
				//edge case that needs to be covered.
				continue
			}
			//handle newEvents
			finished, err = handleEvents(newEvents, parentProcess)
			if err != nil {
				//TODO: panics be bad
				log.Panicf("Unable to handleEvents: %v\n", err)
			}
			if finished {
				return
			}
		}
	}
}

func handleEvents(data []garlic.ProcEvent, parent *TrackedProcess) (bool, error) {
	for _, d := range data {
		printInfo(d)
		// fmt.Printf("%#v\n", data)
		// fmt.Println(fmt.Sprintf("New Data: %v, Type: %v", d.EventData.Pid(), d.WhatString))
		//check if this is a fork
		if t, ok := d.EventData.(garlic.Fork); ok {
			parentPID := uint32(t.ParentPid)
			if process := searchTrackedProcesses(parent, parentPID); process != nil {
				// log.Printf("Found a fork of PID: %v; New PID: %v\n", parentPID, d.EventData.Pid())
				tp := &TrackedProcess{
					PID:   d.EventData.Pid(),
					Start: time.Now(),
				}
				pidInfo, err := ps.FindProcess(int(d.EventData.Pid()))
				if err != nil {
					tp.Name = "unknown"
				} else {
					tp.Name = pidInfo.Executable()
				}
				process.Children = append(process.Children, tp)
			}
			continue
		}

		//check if the pid of d is in our map
		process := searchTrackedProcesses(parent, d.EventData.Pid())
		if process == nil {
			//nope, exit early
			continue
		}
		fmt.Printf("%#v\n", data)
		if d.WhatString == "Exec" {
			log.Printf("Found the launch of process we were expecting with PID: %v\n and Event: %v", d.EventData.Pid(), d.WhatString) //
			continue
		}
		//if it is, then check for exitng
		if d.WhatString == "Exit" {
			// log.Printf("Found the exit of process we were execting with PID: %v\n", d.EventData.Pid())
			// fmt.Printf("%#v\n", data)
			process.End = time.Now()
			process.Done = true
		}
	}

	return checkIfComplete(parent), nil
}

func printInfo(event garlic.ProcEvent) {
	if event.WhatString == "Exit" {
		return
	}
	cmdline := fmt.Sprintf("/proc/%v/cmdline", event.EventData.Pid())
	args := ""
	if Exists(cmdline) {
		d, err := ioutil.ReadFile(cmdline)
		if err == nil {
			args = string(d)
		}
	}
	if t, ok := event.EventData.(garlic.Fork); ok {
		fmt.Printf("Event: %s; PID: %v; ParentPID: %v; TGID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), uint32(t.ParentPid), t.Tgid(), args)
		printInfoByPID("fork", t.ParentPid)
		return
	}
	if t, ok := event.EventData.(garlic.Comm); ok {
		fmt.Printf("Event: %s; PID: %v; Command: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), t.Comm, args)
		return
	}
	if t, ok := event.EventData.(garlic.Exec); ok {
		fmt.Printf("Event: %s; PID: %v; TGID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), t.ProcessTgid, args)
		return
	}

	fmt.Printf("Event: %s; PID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), args)
}

func printInfoByPID(action string, pid uint32) {
	if pid == uint32(1) {
		return
	}
	statFile := fmt.Sprintf("/proc/%v/status", pid)
	stat := ""
	if Exists(statFile) {
		s, err := ioutil.ReadFile(statFile)
		if err == nil {
			stat = string(s)
		}
		if strings.Contains(stat, "Tgid:") && strings.Contains(stat, "PPid:	1") {
			//parse out the Tgid and get pid info by that pid
			parts := strings.Split(stat, "\n")
			for _, v := range parts {
				if strings.Contains(v, "Tgid:") {
					//split 'Tgid: int'
					tgidparts := strings.Split(v, ":")
					tgid := strings.TrimSpace(tgidparts[1])
					fmt.Printf("FOUND A TGID %v\n\n\n\n", tgid)
					if s, err := strconv.Atoi(tgid); err == nil {
						if uint32(s) == pid {
							return
						}
						printInfoByPID("TGIDSPAWN", uint32(s))
					}
				}
			}
		}
	}

	tp := TrackedProcess{}

	pidInfo, err := ps.FindProcess(int(pid))
	if err != nil {
		tp.Name = "unknown"
	}
	if pidInfo != nil {
		tp.Name = pidInfo.Executable()
	}

	//check if pid cmdline info exists
	cmdline := fmt.Sprintf("/proc/%v/cmdline", pid)
	if Exists(cmdline) {
		d, err := ioutil.ReadFile(cmdline)
		if err == nil {
			tp.Args = string(d)
		}
	}

	fmt.Printf("PID: %v - Process %s %s with args %s; stats%s\n", pid, tp.Name, action, tp.Args, stat)
}

// Exists reports whether the named file or directory exists.
//https://stackoverflow.com/a/12527546
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func searchTrackedProcesses(process *TrackedProcess, pid uint32) *TrackedProcess {
	//this is the process we are looking for
	if process.PID == pid {
		return process
	}
	if len(process.Children) == 0 {
		//this process has no child processes
		return nil
	}
	for _, v := range process.Children {
		ret := searchTrackedProcesses(v, pid)
		if ret != nil {
			return ret
		}
	}
	return nil
}

func checkIfComplete(process *TrackedProcess) bool {
	if process == nil {
		return true
	}
	if len(process.Children) > 0 {
		for _, p := range process.Children {
			finished := checkIfComplete(p)
			if !finished {
				return false
			}
		}
	}
	return process.Done
}

func print(process *TrackedProcess) {
	fmt.Println(printProcessTree(process, "", ""))
}

func printProcessTree(process *TrackedProcess, current, output string) string {
	if process == nil {
		return ""
	}
	if len(current) == 0 {
		current = fmt.Sprintf("%s (%v)", process.Name, process.PID)
		output = fmt.Sprintf("%s; Total Time: %v\n", current, process.End.Sub(process.Start))
	} else {
		current = fmt.Sprintf("%s->%s (%v)", current, process.Name, process.PID)
		output = fmt.Sprintf("%s; Total Time: %v\n", current, process.End.Sub(process.Start))
	}
	for _, child := range process.Children {
		output += printProcessTree(child, current, output)
	}
	return output
}
