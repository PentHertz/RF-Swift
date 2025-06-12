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
var Version = "0.6.4"
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
}

func PrintErrorMessage(err error) {
	red := "\033[31m"
	white := "\033[37m"
	reset := "\033[0m"
	fmt.Printf("%s[!] %s%s%s\n", red, white, err.Error(), reset)
}

func PrintSuccessMessage(message string) {
	green := "\033[32m"
	white := "\033[37m"
	reset := "\033[0m"
	fmt.Printf("%s[+] %s%s%s\n", green, white, message, reset)
}

func PrintWarningMessage(message string) {
	yellow := "\033[33m" // Yellow color for warnings or notices
	white := "\033[37m"
	reset := "\033[0m"
	fmt.Printf("%s[!] %s%s%s\n", yellow, white, message, reset)
}

func PrintInfoMessage(message interface{}) {
	blue := "\033[34m"
	reset := "\033[0m"
	fmt.Printf("%s[i] %v%s\n", blue, message, reset)
}

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
