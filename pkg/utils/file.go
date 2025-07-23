package utils

func GetFileType(fileName string) string {
	switch {
	case fileName == "":
		return "unknown"
	case fileName[len(fileName)-4:] == ".txt":
		return "text"
	case fileName[len(fileName)-4:] == ".bin":
		return "binary"
	default:
		return "unknown"
	}
}