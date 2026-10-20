package navigation

// changedMenuFields compares normalized request values with the locked menu.
// A bound permission code belongs to the binding operation, not this field write.
func changedMenuFields(menu Menu, updates map[string]any, boundCode bool) map[string]any {
	var changed map[string]any
	for field, value := range updates {
		equal := false
		switch field {
		case "parent_id":
			equal = menu.ParentID == value.(uint)
		case "name":
			equal = menu.Name == value.(string)
		case "path":
			path := value.(*string)
			equal = menu.Path == nil && path == nil || menu.Path != nil && path != nil && *menu.Path == *path
		case "component":
			equal = menu.Component == value.(string)
		case "icon":
			equal = menu.Icon == value.(string)
		case "permission_code":
			equal = boundCode || menu.PermissionCode == value.(string)
		case "sort":
			equal = menu.Sort == value.(int)
		case "type":
			equal = menu.Type == value.(int)
		case "status":
			equal = menu.Status == value.(int)
		}
		if !equal {
			if changed == nil {
				changed = make(map[string]any, len(updates))
			}
			changed[field] = value
		}
	}
	return changed
}
