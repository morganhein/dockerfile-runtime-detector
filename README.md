# Docker Run Time Inspector

Howdy!

This wasn't as easy as I originally planned. I created a simple runtime detector that worked fine outside of a container. As soon as I threw a container at it though, it failed to give any useful information other than the entire build process.

The reason this occurred, as far as I could research with my limited time, was that the actual "docker build" commands are actually spawned via container/dockerd, and are *not* children of the build process. So, I couldn't just watch the build process for fork/execs. 

Instead, I did something very hacky. It works, and I know there's room for improvement, but i'm not completely satisfied with it. Primarily, I just watch for new processes that get forked by containerd/dockerd (it seemed through testing that both were important). Then, I begin watching those new processes for forks, until I find an exec in the chain who's parent belongs to runcinit. Note that *runcinit* may just by my init system and not yours, so this may not work on your system.

There is lots of other tracking being done as well, so this could also potentially spit out the timings for all the processes forked by the initial build process. However most of this data is how long docker itself takes to perform certain actions preparing an environment for an actual Dockerfile command, and not the command itself.

## Running
Just `sudo go run main.go`.

You can toggle the commented out runlines in main.go to change from the small dockerfile to the one you provided.

## Dependendencies/Environment

Hey, you didn't give me any constraints, so here's what is required for this to work:
* You have to be using Linux. I doubt this works on a Mac, or docker for Mac (although it might)
* The kernel must be built with CONFIG_PROC_EVENTS enabled.
* The init system must be called "runcinit".

All of the above works in Arch Linux with kernel 4.20+

## Cons/Areas of Improvement

* Although I didn't test this, I think that if other containers are being built at the same time, this will pick up those commands, *even if they aren't part of the originally tracked build command*. I feel about 50/50 that this can be solved, however I just didn't have enough time to go down that rabbit hole and figure it out.
* Also, it requires the CONFIG_PROC_EVENTS kernel option to be enabled. This means it may not work in the Mac docker vm.
* It doesn't catch and track "&&" commands, although this could be done.
* Lastly, I don't watch the process tree for docker-in-docker commands, or really any commands that get spawned by the original "docker build" commands, however this definitely could be done fairly easily. 

## Advantages

There is *very little* overhead to this approach, as the entire tracking process is completed by receiving events from the kernel, and the only polling done is triggered by events where new information is known to exist. 

## Sources

* https://bewareofgeek.livejournal.com/2945.html
* https://github.com/fearful-symmetry/garlic
* https://medium.com/@mdlayher/linux-netlink-and-go-part-2-generic-netlink-833bce516400

## Tests

The tests are trash. I abandoned them after I realized the entire workflow had to be reworked. This entire codebase is rather hacky, so it needs to be refactored altogether, let alone making it testable. 