package service

// statusInputs is the data computeLatestStatus reads to derive an
// opportunity's latest_status. Loaded from the DB by the wider events
// engine; kept as a service-local struct so the rule can be
// exhaustively table-tested without standing up a database.
type statusInputs struct {
	archived           bool
	activeAppStatus    string
	latestAppStatus    string
	anyApp             bool
	anyNonPassiveEvent bool
}

// computeLatestStatus derives an opportunity's latest_status from its
// current state. Rules are evaluated top-down; the first match wins:
//
//  1. archived_at set → archived.
//  2. active application (applied/in_progress/offer) → mirror its status.
//  3. latest application is accepted → accepted.
//  4. any application exists but none active → dormant.
//  5. no applications, any non-passive event → exploring.
//  6. otherwise → watching.
//
// Passive event kinds (added, note, follow_up, custom, archived,
// reopened) carry no progression signal and so do not trigger rule 5.
func computeLatestStatus(in statusInputs) string {
	switch {
	case in.archived:
		return "archived"
	case in.activeAppStatus != "":
		return in.activeAppStatus
	case in.latestAppStatus == "accepted":
		return "accepted"
	case in.anyApp:
		return "dormant"
	case in.anyNonPassiveEvent:
		return "exploring"
	default:
		return "watching"
	}
}
