package common

import (
    "fmt"
    "os"
    "os/user"
    "path/filepath"
    "runtime"
    "strings"
)

// RF Swift repo
var Version = "1.0.1"
var Codename = "Skywave"
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

// PrintASCII prints the RF-Swift ASCII art banner to stdout with cycling ANSI
// colors applied line by line, followed by the centered version string.
//
//	out: void
func PrintASCII() {
    colors := []string{
        "\033[31m", // Red
        "\033[33m", // Yellow
        "\033[32m", // Green
        "\033[36m", // Cyan
        "\033[34m", // Blue
        "\033[35m", // Magenta
    }
    reset := "\033[0m"

    lines := strings.Split(ascii_art, "\n")
    for i, line := range lines {
        color := colors[i%len(colors)]
        fmt.Println(color + line + reset)
    }

    // Display version and codename
    cyan := "\033[36m"
    dim := "\033[2m"
    versionStr := fmt.Sprintf("v%s \"%s\"", Version, Codename)
    artWidth := 75 // approximate width of the ASCII art
    pad := (artWidth - len(versionStr)) / 2
    if pad < 0 {
        pad = 0
    }
    fmt.Printf("%s%s%s%s%s\n\n", dim, strings.Repeat(" ", pad), cyan, versionStr, reset)
}

// PrintErrorMessage prints a formatted error message to stdout using red and
// white ANSI colors with a "[!]" prefix.
//
//	in(1): error err the error whose message is to be displayed
//	out: void
func PrintErrorMessage(err error) {
    red := "\033[31m"
    white := "\033[37m"
    reset := "\033[0m"
    fmt.Printf("%s[!] %s%s%s\n", red, white, err.Error(), reset)
}

// PrintSuccessMessage prints a formatted success message to stdout using green
// and white ANSI colors with a "[+]" prefix.
//
//	in(1): string message the success message to display
//	out: void
func PrintSuccessMessage(message string) {
    green := "\033[32m"
    white := "\033[37m"
    reset := "\033[0m"
    fmt.Printf("%s[+] %s%s%s\n", green, white, message, reset)
}

// PrintWarningMessage prints a formatted warning or notice message to stdout
// using yellow and white ANSI colors with a "[!]" prefix.
//
//	in(1): string message the warning message to display
//	out: void
func PrintWarningMessage(message string) {
    yellow := "\033[33m" // Yellow color for warnings or notices
    white := "\033[37m"
    reset := "\033[0m"
    fmt.Printf("%s[!] %s%s%s\n", yellow, white, message, reset)
}

// PrintInfoMessage prints a formatted informational message to stdout using
// blue ANSI color with a "[i]" prefix. The message accepts any type via an
// empty interface so callers can pass strings, errors, or other values.
//
//	in(1): interface{} message the value to display as an informational message
//	out: void
func PrintInfoMessage(message interface{}) {
    blue := "\033[34m"
    reset := "\033[0m"
    fmt.Printf("%s[i] %v%s\n", blue, message, reset)
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