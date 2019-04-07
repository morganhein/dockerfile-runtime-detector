package tracker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	parentProcesses := make([]*TrackedProcess, 0)
	for {
		select {
		case <-cancelChan:
			//TODO: print out the current results? this will be incomplete,
			//and the printProcessTree expects an end time, so this is work to be done.
			return
		case parentProcess := <-parentProcessChan:
			parentProcesses = append(parentProcesses, parentProcess)
			//add this TrackedProcess to be watched by a sentry
			log.Printf("Parent process with PID %v being watched for.\n", parentProcess.PID)
			continue
		case newEvents := <-eventsChan:
			if len(parentProcesses) == 0 {
				//TODO: we don't have a parent process yet, so cache the event for now. This is an
				//edge case that needs to be covered.
				continue
			}
			//handle newEvents
			finished, err = handleEvents(newEvents, parentProcesses)
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

func handleEvents(data []garlic.ProcEvent, parents []*TrackedProcess) (bool, error) {
	for _, d := range data {
		printInfo(d)
		// fmt.Printf("%#v\n", data)
		// fmt.Println(fmt.Sprintf("New Data: %v, Type: %v", d.EventData.Pid(), d.WhatString))
		//check if this is a fork
		if t, ok := d.EventData.(garlic.Fork); ok {
			handleFork(parents, t)
			continue
		}

		//check if the pid of d is in our map
		process := searchTrackedProcesses(parents, d.EventData.Pid())
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

	return checkIfComplete(parents), nil
}

func handleFork(parents []*TrackedProcess, e garlic.Fork) {
	name := getNameByPid(e.Pid())

	//if this is a fork of containerd, begin watching it
	if name != "containerd" {
		tp := &TrackedProcess{
			PID:   e.Pid(),
			Start: time.Now(),
			Name:  name,
			//TODO: Args,
		}
		parents = append(parents, tp)
		return
	}

	//otherwise check if this is a fork of a process we are already watching
	parentPID := uint32(e.ParentPid)
	if process := searchTrackedProcesses(parents, parentPID); process != nil {
		// log.Printf("Found a fork of PID: %v; New PID: %v\n", parentPID, d.EventData.Pid())
		tp := &TrackedProcess{
			PID:   e.Pid(),
			Start: time.Now(),
			Name:  name,
		}
		process.Children = append(process.Children, tp)
	}
}

//searchTrackedProcesses searches all of the process trees we know about. Currently I can't find a way to track
//the containerd forks that eventually execute commands in the dockerfile from the `docker build` command.
//Instead I just capture all containderd forks that lead to spawns, and they are each an individual tree that gets
//searched for.
func searchTrackedProcesses(processes []*TrackedProcess, pid uint32) *TrackedProcess {
	for _, ps := range processes {
		found := searchTrackedProcessesHelper(ps, pid)
		if found != nil {
			return found
		}
	}
	return nil
}

func searchTrackedProcessesHelper(process *TrackedProcess, pid uint32) *TrackedProcess {
	//this is the process we are looking for
	if process.PID == pid {
		return process
	}
	if len(process.Children) == 0 {
		//this process has no child processes
		return nil
	}
	for _, v := range process.Children {
		ret := searchTrackedProcessesHelper(v, pid)
		if ret != nil {
			return ret
		}
	}
	return nil
}

func checkIfComplete(processes []*TrackedProcess) bool {
	if len(processes) == 0 {
		return true
	}
	for _, ps := range processes {
		finished := checkIfCompleteHelper(ps)
		if !finished {
			return false
		}
	}
	return true
}

func checkIfCompleteHelper(process *TrackedProcess) bool {
	if process == nil {
		return true
	}
	if len(process.Children) > 0 {
		for _, p := range process.Children {
			finished := checkIfCompleteHelper(p)
			if !finished {
				return false
			}
		}
	}
	return true
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
