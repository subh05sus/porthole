# VHS tape script for porthole's README demo GIF.
# https://github.com/charmbracelet/vhs
#
# NOT rendered — vhs, and its own runtime dependencies (ttyd, ffmpeg),
# are not installed in this dev environment, and setting all three up
# was out of scope for this session. Once vhs is installed, render with:
#
#   vhs demo.tape
#
# which produces demo.gif for the README.

Output demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 60ms

Type "porthole"
Sleep 500ms
Enter
Sleep 2s

# Let the TUI's own animated reveal play out.
Sleep 1s

# Filter down to something specific.
Type "/"
Sleep 300ms
Type "node"
Sleep 1s
Escape
Sleep 500ms

# Open the detail pane on the selected row.
Enter
Sleep 2s
Escape
Sleep 500ms

# Show the help overlay.
Type "?"
Sleep 2s
Escape
Sleep 500ms

# Quit.
Type "q"
Sleep 1s
