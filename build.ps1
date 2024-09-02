# thank you b7
$Name="Gamba-Suite"
echo "Building for Linux..."
$env:GOOS="windows"; wails build -o bin/${Name}-win.exe .
