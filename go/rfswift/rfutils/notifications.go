package rfutils

import (
	common "penthertz/rfswift/common"
)

// DisplayNotification renders a formatted notification box in the terminal with a
// title, message body, and visual style determined by the notification type.
//
//	in(1): string title        the heading displayed at the top of the box
//	in(2): string message      the body text; newlines produce multiple wrapped rows
//	in(3): string notificationType  style selector: "warning", "error", "info", "success"
func DisplayNotification(title string, message string, notificationType string) {
	switch notificationType {
	case "error":
		common.PrintErrorMessage(formatNotifError{body: message})
	case "warning":
		common.PrintWarningMessage(message)
	case "info":
		common.PrintInfoBox(message)
	case "success":
		common.PrintSuccessMessage(message)
	default:
		common.PrintInfoBox(message)
	}
}

// formatNotifError implements the error interface to pass to PrintErrorMessage.
type formatNotifError struct {
	body string
}

func (e formatNotifError) Error() string {
	return e.body
}
