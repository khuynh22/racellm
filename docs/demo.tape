# RaceLLM demo tape — record with: vhs docs/demo.tape
# Install VHS: go install github.com/charmbracelet/vhs@latest

Output docs/demo.gif

Set Shell "powershell"
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Dracula"

# Show help
Type ".\racellm.exe --help"
Enter
Sleep 2s

# Clear and run a real race
Type "cls"
Enter
Sleep 300ms

Type `.\racellm.exe "What is the speed of light?" --mode fastest`
Enter

# Let the race play out — adjust if your models are slower
Sleep 15s
