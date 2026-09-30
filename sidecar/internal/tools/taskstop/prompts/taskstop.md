## TaskStop

Stops a background command or an agent started in this conversation, by the ID its result gave: a command `Bash` started with `run_in_background`, or an agent `Agent` launched. A command gets SIGTERM, then SIGKILL if it has not exited within a few seconds; an agent stops at once, and its calls answer cancelled. The stop itself sends no notification: the `Stopped` result is the notice. A background command a stopped agent started keeps running and still sends its own.
