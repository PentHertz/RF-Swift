package common

import (
    "fmt"
    "os"
    "os/user"
    "path/filepath"
    "runtime"
    "strings"

    "github.com/charmbracelet/lipgloss"
    "golang.org/x/term"
)

// RF Swift repo
var Version = "2.2.1"
var Codename = "Harmonic"
var Branch = "main"
var Owner = "PentHertz"
var Repo = "RF-Swift"

var Disconnected bool = false // variable avoiding checks for updates

var ascii_art_old = `                                                                                                    
                                                                                                                                                      
                  -%@%-                     :==-.                                                                                                     
                :%:*+=:+%.             .%@%#%+-:-=%@*                                                                                                 
              %==#=: .  .+#          :@%:*==--:   .  =%.                                                                                              
            :@=::+#:.  .+-:%.       %@:*+==..=**+=-:::=#=                                                                                             
           +****=:+. +=::*#-%-    .@+:*.:.:=***:.::...:*#*                                                                                            
          ::%*#*:* :+- *:-.=%**. =@:+*+:.==.:::=::   .-:.:#+                                                                                          
          :*%#**%.-%%%+ :#:#+:.=#%%@@*.:==++.=:.-#@@@#:*==+=%                                                                                         
           %#@+*:       +%##+%@@@%=...==-+= ::%@*#@%#%%%@%=*=*                                                                                        
          :*#%#*.     .=: **@%%%%#==+=-=+: .+@:   %*+%=...*@+@:                                                                                       
           +=*+*@:   ::. .@*@@@@#*+*##%*::=@*    :%*-:-%*.  *@:                                                                                       
           ::.@=@=#=  . =@@@@%=*#*%@%===+%@+=*%*:*%=%.--*#-: %*                                                                                       
              ##@:==%%%@@@@#=#%#*-:-=*%@@@%+:::=%%=#-:-:-@:= .@                                                                                       
              .%@. :==@@@%#@@@*=*#===:: .:=+***=+**.::-:-%=+ :@                                                                                       
               *@:    %%@@@%+=*@@@%%%*====-::.    ::=-=:*%=-.@*                                                                                       
          .*%%%*=.   :@%:.  .*%=::*#@@@@%*=::::::::..:*@#-:.%%.                                                                                       
         **  =%.-  .  @@      :@%*=:::==+**%%@@@@@@@%*=:..*@*                                                                                         
         %: =@:       %@:      .@+%@%#==--::::----:::.:*@*:=#%@=                                                                                      
         -%*%.        :@* :.    :@.  :=%@@%%%#**#%%@@#-:#*:   %=                                                                                      
         .@=   ..      #@-**.    -@:        .#*#%.  :%=.    :@*                                                                                       
        :%.   --     =++@@=+*.    :%.       +**@@%@+-:::*  %@@.                                                                                       
        %@-  -*   =%@@@@@@%=**:    =%   :#@@=%@@@@-   .%.%@@*                                                                                         
        @@@+.=*:-@@@@%%%@%@%%@#:   :#@@=.   *#.:-@#   *@@@@%                                                                                          
        %@%@%=**@@%%%%@@@@@@@*.=@#:  .*%:+%*-    *@%@@@@@:*=                                                                                          
        :@@@@@#@@@@@@@@@@= :*:  :*@@=  ** *+====-@@@@@#. *=                                                                                           
          :@@@@@@@@@@+     .=%@@-    -#     :#@@@@@@@@--%.                                                                                            
           :@@@@@#:.:- .=@@%=       .%%%@@@@@@@*: :%@@%.                                                                                              
            #@*.:=-:=%@@*...:::::..         .=+***=#@%                                                                                                
            -@=--*@@@*            :=+#%%#+::       #@@=%:                                                                                             
            .@*%@@*.                        .-+=:.:@@@* -#=.                                                                                          
             *@%=                                          .:                                                                                         
              :                                                                                                                                                                                                    
                                     
`
var ascii_art = `

    888~-_   888~~        ,d88~~\                ,e,   88~\   d8   
    888   \  888___       8888    Y88b    e    /  "  _888__ _d88__ 
    888    | 888          'Y88b    Y88b  d8b  /  888  888    888   
    888   /  888           'Y88b,   Y888/Y88b/   888  888    888   
    888_-~   888             8888    Y8/  Y8/    888  888    888   
    888 ~-_  888          \__88P'     Y    Y     888  888    "88_/       

                RF toolbox for HAMs and professionals                                                                             
`

// termWidth returns the current terminal width, defaulting to 80.
func termWidth() int {
    w, _, err := term.GetSize(int(os.Stdout.Fd()))
    if err != nil || w <= 0 {
        return 80
    }
    return w
}

// PrintASCII prints the RF-Swift ASCII art banner to stdout with cycling
// colors applied line by line, followed by the centered version string.
func PrintASCII() {
    colors := []lipgloss.Color{
        lipgloss.Color("#FF4444"), // Red
        lipgloss.Color("#FFAA00"), // Yellow
        lipgloss.Color("#00FF00"), // Green
        lipgloss.Color("#00FFFF"), // Cyan
        lipgloss.Color("#4488FF"), // Blue
        lipgloss.Color("#FF69B4"), // Magenta
    }

    lines := strings.Split(ascii_art, "\n")
    for i, line := range lines {
        s := lipgloss.NewStyle().Foreground(colors[i%len(colors)])
        fmt.Println(s.Render(line))
    }

    // Display version and codename
    versionStr := fmt.Sprintf("v%s \"%s\"", Version, Codename)
    artWidth := 75
    pad := (artWidth - len(versionStr)) / 2
    if pad < 0 {
        pad = 0
    }
    styled := lipgloss.NewStyle().
        Foreground(lipgloss.Color("#00FFFF")).
        Faint(true).
        Render(strings.Repeat(" ", pad) + versionStr)
    fmt.Printf("%s\n\n", styled)
}

// messageBox renders a styled message box with a title line and wrapped body.
func messageBox(icon string, title string, body string, borderColor lipgloss.Color, titleColor lipgloss.Color) {
    w := termWidth()
    boxWidth := w - 4
    if boxWidth < 30 {
        boxWidth = 30
    }
    if boxWidth > 100 {
        boxWidth = 100
    }

    innerWidth := boxWidth - 4 // account for "│ " and " │"

    border := lipgloss.NewStyle().Foreground(borderColor)
    titleStyle := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

    // Top border
    fmt.Printf("%s\n", border.Render("┌"+strings.Repeat("─", boxWidth-2)+"┐"))

    // Title line — use lipgloss.Width for correct Unicode display width
    titleText := fmt.Sprintf(" %s %s", icon, title)
    padded := titleText + strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(titleText)))
    fmt.Printf("%s%s%s\n", border.Render("│ "), titleStyle.Render(padded), border.Render(" │"))

    // Separator
    fmt.Printf("%s\n", border.Render("├"+strings.Repeat("─", boxWidth-2)+"┤"))

    // Body lines with word wrapping
    bodyLines := strings.Split(body, "\n")
    for _, line := range bodyLines {
        wrapped := wrapTextSimple(line, innerWidth)
        for _, wl := range wrapped {
            padLine := wl + strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(wl)))
            fmt.Printf("%s%s%s\n", border.Render("│ "), padLine, border.Render(" │"))
        }
    }

    // Bottom border
    fmt.Printf("%s\n", border.Render("└"+strings.Repeat("─", boxWidth-2)+"┘"))
}

// wrapTextSimple splits text into lines that fit within width characters.
func wrapTextSimple(text string, width int) []string {
    if width <= 0 {
        return []string{text}
    }
    words := strings.Fields(text)
    if len(words) == 0 {
        return []string{""}
    }

    var lines []string
    current := ""
    for _, word := range words {
        if current == "" {
            current = word
        } else if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
            current += " " + word
        } else {
            lines = append(lines, current)
            current = word
        }
    }
    if current != "" {
        lines = append(lines, current)
    }
    return lines
}

// stripAnsiSimple removes ANSI escape sequences for length calculation.
func stripAnsiSimple(s string) string {
    result := s
    for {
        start := strings.Index(result, "\033[")
        if start == -1 {
            break
        }
        end := strings.IndexByte(result[start:], 'm')
        if end == -1 {
            break
        }
        result = result[:start] + result[start+end+1:]
    }
    return result
}

// PrintErrorMessage prints a styled error box.
func PrintErrorMessage(err error) {
    red := lipgloss.Color("#FF4444")
    messageBox("✗", "Error", err.Error(), red, red)
}

// PrintSuccessMessage prints a styled success box.
func PrintSuccessMessage(message string) {
    green := lipgloss.Color("#00FF00")
    messageBox("✓", "Success", message, green, green)
}

// PrintWarningMessage prints a styled warning box.
func PrintWarningMessage(message string) {
    yellow := lipgloss.Color("#FFAA00")
    messageBox("!", "Warning", message, yellow, yellow)
}

// PrintInfoMessage prints a styled informational message with a "[i]" prefix.
func PrintInfoMessage(message interface{}) {
    blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#4488FF"))
    fmt.Printf("%s %v\n", blue.Render("[i]"), message)
}

// PrintInfoBox prints an informational message in a styled box.
func PrintInfoBox(message string) {
    cyan := lipgloss.Color("#00BFFF")
    messageBox("i", "Info", message, cyan, cyan)
}

// ConfigFileByPlatform returns the platform-appropriate absolute path to the
// rfswift configuration file. On Linux it resolves to
// ~/.config/rfswift/config.ini, on macOS to
// ~/Library/Application Support/rfswift/config.ini, and on Windows to
// %APPDATA%\rfswift\config.ini. When the process is running under sudo the
// home directory of the original user (SUDO_USER) is used instead of root's.
//
//	out: string absolute path to the platform-specific config file
func ConfigFileByPlatform() string {
    var configPath string

    // Get the current user, handling cases where sudo is used
    homeDir := os.Getenv("HOME") // Default to the HOME environment variable
    if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
        // Use SUDO_USER to find the original user's home directory
        userInfo, err := user.Lookup(sudoUser)
        if err == nil {
            homeDir = userInfo.HomeDir
        }
    }

    // Determine the platform-specific directory
    switch runtime.GOOS {
    case "windows":
        configPath = filepath.Join(os.Getenv("APPDATA"), "rfswift", "config.ini")
    case "darwin":
        configPath = filepath.Join(homeDir, "Library", "Application Support", "rfswift", "config.ini")
    case "linux":
        configPath = filepath.Join(homeDir, ".config", "rfswift", "config.ini")
    default:
        configPath = "config.ini"
    }
    return configPath
}
