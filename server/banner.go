package server

import "strings"

const namePrompt = "[ENTER YOUR NAME]: "

var linuxLogo = strings.Join([]string{
	"         _nnnn_",
	"        dGGGGMMb",
	"       @p~qp~~qMb",
	"       M|@||@) M|",
	"       @,----.JM|",
	"      JS^\\__/  qKL",
	"     dZP        qKRb",
	"    dZP          qKKb",
	"   fZP            SMMb",
	"   HZM            MMMM",
	"   FqM            MMMM",
	" __| \".        |\\dS\"qML",
	" |    `.       | `' \\Zq",
	"_)      \\.___.,|     .'",
	"\\____   )MMMMMP|   .'",
	"     `-'       `--'",
}, "\n")

// welcomeBanner is the first thing every new connection receives.
var welcomeBanner = "Welcome to TCP-Chat!\n" + linuxLogo + "\n"
