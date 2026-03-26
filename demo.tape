# podspawn demo
#
# Pre-cache image: podspawn dev -- true (in a temp dir with the go template)
# Record: vhs demo.tape

Output demo.gif
Output demo.webm

Require podspawn

Set Shell "zsh"
Set Theme "Catppuccin Mocha"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 17
Set Width 1400
Set Height 800
Set Padding 24
Set Framerate 30
Set TypingSpeed 45ms
Set CursorBlink false
Set WindowBar Colorful
Set WindowBarSize 40

# --- Setup (hidden) ---
Hide
Type "cd /tmp && rm -rf vhs-demo && mkdir vhs-demo && cd vhs-demo"
Enter
Sleep 500ms
Show

Sleep 600ms

# --- Scaffold a Go project ---
Type "podspawn init -y --template go"
Sleep 400ms
Enter
Sleep 2s

# --- Show the Podfile ---
Type "cat podfile.yaml"
Sleep 300ms
Enter
Sleep 5s

# --- Start the dev environment (cold start) ---
Type "podspawn dev -- go version"
Sleep 400ms
Enter
Sleep 8s

# --- Re-attach: show its Linux ---
Type "podspawn dev -- uname -a"
Sleep 300ms
Enter
Sleep 4s

# --- Show the workspace mount ---
Type "podspawn dev -- ls /workspace/vhs-demo"
Sleep 300ms
Enter
Sleep 4s

# --- List running machines ---
Type "podspawn list"
Sleep 300ms
Enter
Sleep 3s

# --- Teardown ---
Type "podspawn down"
Sleep 300ms
Enter
Sleep 3s

Sleep 1s
