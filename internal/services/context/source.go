package contextsvc

func isAnchor(kind string) bool {
	return kind == "direct_user" || kind == "delegated_task" || kind == "human_input"
}
