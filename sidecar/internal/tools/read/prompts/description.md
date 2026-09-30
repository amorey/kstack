Reads a text file from the user's machine.

- `file_path` must be an absolute path in its plain form: no `.` or `..` components, and no `~`.
- A file Kstack saved for this conversation, such as a command's full output, is read at once. Any other file waits for the user to approve it.
- Reads up to 2000 lines from `offset` (1-based, default 1). Lines longer than 2000 characters are cut. When you already know which part of the file you need, read only that part.
- Results are returned with line numbers, as `cat -n` prints them.
- Reads text only: not images, PDFs, binaries or directories. A symbolic link is refused with its target; read the target.
- Kstack's own directories cannot be read.
