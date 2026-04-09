# RaceLLM demo tape — record with: vhs demo.tape
# Install VHS: https://github.com/charmbracelet/vhs

Output docs/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Dracula"

# Show the help briefly
Type "racellm --help"
Enter
Sleep 2s

# Clear and run a real race
Ctrl+L
Sleep 500ms

Type `racellm "What is the speed of light?" --mode fastest`
Enter

# Let the race play out
Sleep 8s
