package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	appName = "gdocsync"

	credentialsFile = "credentials.json"
	tokenFile       = "token.json"
	stateFileName   = "gdocsync-state.json"

	defaultOutput = "output"

	// IMPORTANT:
	// This application can only read/download Drive content.
	Scope = drive.DriveReadonlyScope

	googleDocMime = "application/vnd.google-apps.document"
	docxMime      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	folderMime    = "application/vnd.google-apps.folder"
)

var (
	version = "1.0.0"

	ErrNotFound = errors.New("not found")

	reEmptyLines = regexp.MustCompile(`\n{3,}`)
	reDigits     = regexp.MustCompile(`\d+`)
)

/* -------------------------------------------------------------------------- */
/* APP DIRECTORY                                                             */
/* -------------------------------------------------------------------------- */

// appDir is the directory where app-level JSON files
// (credentials.json, token.json, gdocsync-state.json) are read from and
// written to. It prefers the directory of the running executable; if
// credentials.json is not present there, it falls back to the current
// working directory (useful for `go run .`).
var appDir = detectAppDir()

func detectAppDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, credentialsFile)); err == nil {
			return dir
		}
	}
	return "."
}

func appFile(name string) string {
	return filepath.Join(appDir, name)
}

/* -------------------------------------------------------------------------- */
/* TYPES                                                                     */
/* -------------------------------------------------------------------------- */

type SyncState struct {
	Version int                  `json:"version"`
	Files   map[string]FileState `json:"files"`
}

type FileState struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ModifiedTime string `json:"modified_time"`
	Output       string `json:"output"`
}

type DriveDocument struct {
	ID           string
	Name         string
	ModifiedTime string
	MimeType     string
}

/* -------------------------------------------------------------------------- */
/* MAIN                                                                      */
/* -------------------------------------------------------------------------- */

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "auth":
		cmdAuth(os.Args[2:])

	case "list":
		cmdList(os.Args[2:])

	case "sync":
		cmdSync(os.Args[2:])

	case "convert":
		cmdConvert(os.Args[2:])

	case "logout":
		cmdLogout()

	case "version":
		fmt.Printf("%s %s\n", appName, version)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`%s %s

Sync Google Docs from Google Drive into Markdown.

Usage:

  %s auth
  %s list <folder-url-or-id>
  %s sync <folder-url-or-id> [options]
  %s convert <input-dir> [options]
  %s logout
  %s version

Commands:

  auth
      Authenticate with Google Drive.

  list
      List Google Docs inside a Drive folder.

  sync
      Download Google Docs from a Drive folder as .docx files.
      Records downloaded files in a state file inside the app directory.

  convert
      Convert .docx files in a directory to Markdown using Pandoc.
      This command does not access Google Drive.

  logout
      Remove the locally stored OAuth token.

  version
      Show version information.

Sync options:

  --output <dir>
      Output directory for downloaded .docx files.
      Default: %s

  --force
      Re-download every document even if unchanged.

  --dry-run
      Show what would be downloaded without writing files.

Convert options:

  --output <dir>
      Output directory for Markdown files.
      Default: same as input directory.

  --force
      Reconvert every file even if the Markdown is up to date.

Notes:

  Flags may be placed before or after positional arguments, e.g.:

      gdocsync convert chapters --output markdown
      gdocsync sync ABC123 --force

  credentials.json, token.json and gdocsync-state.json are stored in the
  directory of the running executable (falling back to the current working
  directory during "go run .").

Examples:

  %s auth
  %s list "https://drive.google.com/drive/folders/ABC123"
  %s sync "https://drive.google.com/drive/folders/ABC123"
  %s sync ABC123 --output chapters
  %s convert chapters
  %s convert chapters --output markdown
  %s sync ABC123 --force
  %s sync ABC123 --dry-run
  
`, appName, version,
		appName,
		appName,
		appName,
		appName,
		appName,
		appName,
		defaultOutput,
		appName,
		appName,
		appName,
		appName,
		appName,
		appName,
		appName,
		appName,
	)
}

/* -------------------------------------------------------------------------- */
/* FLAG HELPERS                                                              */
/* -------------------------------------------------------------------------- */

// reorderArgs moves all flag arguments before positional arguments so that
// flags may safely appear after positional arguments. Without this, Go's
// standard flag package stops parsing at the first non-flag argument, which
// would silently ignore things like "convert chapters --output markdown".
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	boolFlags := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if bv, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})

	var flags, positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}

		if len(arg) > 1 && strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)

			name := strings.TrimLeft(arg, "-")
			if idx := strings.Index(name, "="); idx >= 0 {
				continue
			}
			if boolFlags[name] {
				continue
			}
			// Value-taking flag: consume the next argument.
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}

		positionals = append(positionals, arg)
	}

	return append(flags, positionals...)
}

/* -------------------------------------------------------------------------- */
/* AUTH                                                                      */
/* -------------------------------------------------------------------------- */

func cmdAuth(args []string) {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	fs.Parse(reorderArgs(fs, args))

	ctx := context.Background()

	fmt.Println("gdocsync authentication")
	fmt.Println("=======================")
	fmt.Println()

	client, err := createGoogleClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	service, err := drive.NewService(
		ctx,
		option.WithHTTPClient(client),
	)

	if err != nil {
		log.Fatal(err)
	}

	_, err = service.About.Get().
		Fields("user(displayName,emailAddress)").
		Do()

	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	fmt.Println()
	fmt.Println("Authentication successful.")
	fmt.Printf("Token saved to %s\n", appFile(tokenFile))
	fmt.Println()
	fmt.Println("Drive access: READ ONLY")
}

/* -------------------------------------------------------------------------- */
/* LIST                                                                      */
/* -------------------------------------------------------------------------- */

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Parse(reorderArgs(fs, args))

	if fs.NArg() < 1 {
		fmt.Println("Usage: gdocsync list <folder-url-or-id>")
		os.Exit(1)
	}

	folderID, err := extractFolderID(fs.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	client, err := createGoogleClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	service, err := drive.NewService(
		ctx,
		option.WithHTTPClient(client),
	)

	if err != nil {
		log.Fatal(err)
	}

	folder, err := getFolder(ctx, service, folderID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Folder: %s\n", folder.Name)
	fmt.Printf("ID:     %s\n\n", folder.Id)

	docs, err := listDocuments(ctx, service, folderID)
	if err != nil {
		log.Fatal(err)
	}

	if len(docs) == 0 {
		fmt.Println("No Google Docs found.")
		return
	}

	sortDocuments(docs)

	for i, doc := range docs {
		fmt.Printf(
			"%03d  %-60s  %s\n",
			i+1,
			doc.Name,
			doc.ModifiedTime,
		)
	}

	fmt.Printf("\n%d document(s).\n", len(docs))
}

/* -------------------------------------------------------------------------- */
/* SYNC (download .docx from Drive)                                          */
/* -------------------------------------------------------------------------- */

func cmdSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)

	outputDir := fs.String(
		"output",
		defaultOutput,
		"output directory for downloaded .docx files",
	)

	force := fs.Bool(
		"force",
		false,
		"re-download all files",
	)

	dryRun := fs.Bool(
		"dry-run",
		false,
		"show changes without writing files",
	)

	fs.Parse(reorderArgs(fs, args))

	if fs.NArg() < 1 {
		fmt.Println("Usage: gdocsync sync <folder-url-or-id> [options]")
		os.Exit(1)
	}

	folderID, err := extractFolderID(fs.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)

	defer stop()

	client, err := createGoogleClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	service, err := drive.NewService(
		ctx,
		option.WithHTTPClient(client),
	)

	if err != nil {
		log.Fatal(err)
	}

	folder, err := getFolder(ctx, service, folderID)
	if err != nil {
		log.Fatal(err)
	}

	docs, err := listDocuments(ctx, service, folderID)
	if err != nil {
		log.Fatal(err)
	}

	sortDocuments(docs)

	fmt.Println("gdocsync sync")
	fmt.Println("=============")
	fmt.Printf("Folder:  %s\n", folder.Name)
	fmt.Printf("Files:   %d\n", len(docs))
	fmt.Printf("Output:  %s\n", *outputDir)
	fmt.Printf("State:   %s\n", appFile(stateFileName))
	fmt.Printf("Access:  READ ONLY\n")
	fmt.Println()

	statePath := appFile(stateFileName)

	state, err := loadState(statePath)
	if err != nil {
		log.Fatal(err)
	}

	if !*dryRun {
		if err := os.MkdirAll(*outputDir, 0755); err != nil {
			log.Fatalf("Unable to create output directory: %v", err)
		}
	}

	var downloaded int
	var skipped int
	var failed int

	for index, doc := range docs {
		fmt.Printf(
			"[%03d/%03d] %s\n",
			index+1,
			len(docs),
			doc.Name,
		)

		outputName := makeOutputName(doc.Name, ".docx")
		outputPath := filepath.Join(*outputDir, outputName)

		previous, exists := state.Files[doc.ID]

		unchanged := exists &&
			previous.ModifiedTime == doc.ModifiedTime &&
			previous.Output == outputName &&
			fileExists(outputPath)

		if unchanged && !*force {
			fmt.Println("         unchanged")
			skipped++
			continue
		}

		if *dryRun {
			if exists {
				fmt.Println("         would update")
			} else {
				fmt.Println("         would download")
			}

			downloaded++
			continue
		}

		if err := exportDocument(
			ctx,
			service,
			doc.ID,
			outputPath,
		); err != nil {
			fmt.Printf("         ERROR: %v\n", err)
			failed++
			continue
		}

		state.Files[doc.ID] = FileState{
			ID:           doc.ID,
			Name:         doc.Name,
			ModifiedTime: doc.ModifiedTime,
			Output:       outputName,
		}

		downloaded++

		fmt.Println("         downloaded")
	}

	if !*dryRun {
		if err := saveState(statePath, state); err != nil {
			log.Fatalf("Unable to save state: %v", err)
		}
	}

	fmt.Println()
	fmt.Println("Sync complete.")
	fmt.Printf("  Downloaded: %d\n", downloaded)
	fmt.Printf("  Skipped:    %d\n", skipped)
	fmt.Printf("  Failed:     %d\n", failed)

	if failed > 0 {
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/* CONVERT (.docx → .md)                                                     */
/* -------------------------------------------------------------------------- */

func cmdConvert(args []string) {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)

	outputDir := fs.String(
		"output",
		"",
		"output directory for Markdown files (default: input directory)",
	)

	force := fs.Bool(
		"force",
		false,
		"reconvert all files",
	)

	fs.Parse(reorderArgs(fs, args))

	if fs.NArg() < 1 {
		fmt.Println("Usage: gdocsync convert <input-dir> [options]")
		os.Exit(1)
	}

	inputDir := fs.Arg(0)

	if *outputDir == "" {
		*outputDir = inputDir
	}

	if err := checkPandoc(); err != nil {
		log.Fatal(err)
	}

	info, err := os.Stat(inputDir)
	if err != nil {
		log.Fatalf("Cannot read input directory %q: %v", inputDir, err)
	}
	if !info.IsDir() {
		log.Fatalf("Input %q is not a directory", inputDir)
	}

	docxFiles, err := findDocxFiles(inputDir)
	if err != nil {
		log.Fatal(err)
	}

	if len(docxFiles) == 0 {
		fmt.Printf("No .docx files found in %s\n", inputDir)
		return
	}

	fmt.Println("gdocsync convert")
	fmt.Println("================")
	fmt.Printf("Input:   %s\n", inputDir)
	fmt.Printf("Output:  %s\n", *outputDir)
	fmt.Printf("Files:   %d\n", len(docxFiles))
	fmt.Println()

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Unable to create output directory: %v", err)
	}

	var converted int
	var skipped int
	var failed int

	for index, name := range docxFiles {
		fmt.Printf(
			"[%03d/%03d] %s\n",
			index+1,
			len(docxFiles),
			name,
		)

		inputPath := filepath.Join(inputDir, name)

		outputName := strings.TrimSuffix(name, filepath.Ext(name)) + ".md"
		outputPath := filepath.Join(*outputDir, outputName)

		if !*force && isNewerOrEqual(outputPath, inputPath) {
			fmt.Println("         unchanged")
			skipped++
			continue
		}

		err := convertDocxToMarkdown(
			inputPath,
			outputPath,
		)

		if err != nil {
			fmt.Printf("         ERROR: %v\n", err)
			failed++
			continue
		}

		converted++
		fmt.Println("         converted")
	}

	fmt.Println()
	fmt.Println("Convert complete.")
	fmt.Printf("  Converted: %d\n", converted)
	fmt.Printf("  Skipped:   %d\n", skipped)
	fmt.Printf("  Failed:    %d\n", failed)

	if failed > 0 {
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/* DOCX → MARKDOWN                                                           */
/* -------------------------------------------------------------------------- */

func convertDocxToMarkdown(
	docxPath string,
	outputPath string,
) error {

	tempDir, err := os.MkdirTemp("", "gdocsync-*")
	if err != nil {
		return err
	}

	defer os.RemoveAll(tempDir)

	rawMarkdownPath := filepath.Join(
		tempDir,
		"document.md",
	)

	if err := convertWithPandoc(
		docxPath,
		rawMarkdownPath,
	); err != nil {
		return err
	}

	content, err := os.ReadFile(rawMarkdownPath)
	if err != nil {
		return err
	}

	content = cleanMarkdown(content)

	return atomicWrite(
		outputPath,
		content,
	)
}

/* -------------------------------------------------------------------------- */
/* GOOGLE DRIVE                                                              */
/* -------------------------------------------------------------------------- */

func listDocuments(
	ctx context.Context,
	service *drive.Service,
	folderID string,
) ([]DriveDocument, error) {

	query := fmt.Sprintf(
		"'%s' in parents and mimeType = '%s' and trashed = false",
		escapeDriveQuery(folderID),
		googleDocMime,
	)

	var documents []DriveDocument

	pageToken := ""

	for {
		call := service.Files.List().
			Q(query).
			PageSize(100).
			Fields(
				"nextPageToken,files(id,name,mimeType,modifiedTime)",
			)

		if pageToken != "" {
			call.PageToken(pageToken)
		}

		result, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf(
				"unable to list Drive files: %w",
				err,
			)
		}

		for _, file := range result.Files {
			documents = append(
				documents,
				DriveDocument{
					ID:           file.Id,
					Name:         file.Name,
					ModifiedTime: file.ModifiedTime,
					MimeType:     file.MimeType,
				},
			)
		}

		pageToken = result.NextPageToken

		if pageToken == "" {
			break
		}
	}

	return documents, nil
}

func getFolder(
	ctx context.Context,
	service *drive.Service,
	folderID string,
) (*drive.File, error) {

	file, err := service.Files.Get(folderID).
		Fields("id,name,mimeType").
		Do()

	if err != nil {
		return nil, fmt.Errorf(
			"unable to access folder %s: %w",
			folderID,
			err,
		)
	}

	if file.MimeType != folderMime {
		return nil, fmt.Errorf(
			"%s is not a Google Drive folder",
			folderID,
		)
	}

	return file, nil
}

func exportDocument(
	ctx context.Context,
	service *drive.Service,
	fileID string,
	outputPath string,
) error {

	response, err := service.Files.Export(
		fileID,
		docxMime,
	).Download()

	if err != nil {
		return fmt.Errorf(
			"Drive export failed: %w",
			err,
		)
	}

	defer response.Body.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)

	if err != nil {
		return fmt.Errorf(
			"failed writing exported document: %w",
			err,
		)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/* PANDOC                                                                    */
/* -------------------------------------------------------------------------- */

func checkPandoc() error {
	_, err := exec.LookPath("pandoc")

	if err != nil {
		return errors.New(
			"Pandoc was not found in PATH; install Pandoc first",
		)
	}

	return nil
}

func convertWithPandoc(
	inputPath string,
	outputPath string,
) error {

	cmd := exec.Command(
		"pandoc",
		inputPath,
		"--from=docx",
		"--to=gfm",
		"--wrap=none",
		"--output="+outputPath,
	)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf(
			"Pandoc failed: %v\n%s",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/* MARKDOWN                                                                  */
/* -------------------------------------------------------------------------- */

func cleanMarkdown(data []byte) []byte {
	text := strings.ReplaceAll(
		string(data),
		"\r\n",
		"\n",
	)

	text = strings.TrimSpace(text)

	// Google Docs/Pandoc can occasionally produce excessive
	// empty lines. Keep at most two.
	text = reEmptyLines.ReplaceAllString(text, "\n\n")

	return []byte(text + "\n")
}

/* -------------------------------------------------------------------------- */
/* STATE                                                                     */
/* -------------------------------------------------------------------------- */

func loadState(path string) (SyncState, error) {
	state := SyncState{
		Version: 1,
		Files:   make(map[string]FileState),
	}

	data, err := os.ReadFile(path)

	if os.IsNotExist(err) {
		return state, nil
	}

	if err != nil {
		return state, err
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf(
			"invalid %s: %w",
			path,
			err,
		)
	}

	if state.Files == nil {
		state.Files = make(map[string]FileState)
	}

	return state, nil
}

func saveState(path string, state SyncState) error {
	data, err := json.MarshalIndent(
		state,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	return atomicWrite(
		path,
		append(data, '\n'),
	)
}

/* -------------------------------------------------------------------------- */
/* FILES                                                                     */
/* -------------------------------------------------------------------------- */

func atomicWrite(
	path string,
	data []byte,
) error {

	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(
		dir,
		".gdocsync-*",
	)

	if err != nil {
		return err
	}

	tempName := temp.Name()

	defer os.Remove(tempName)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempName, path); err != nil {
		return err
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

func isNewerOrEqual(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}

	bi, err := os.Stat(b)
	if err != nil {
		return false
	}

	return !ai.ModTime().Before(bi.ModTime())
}

func findDocxFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.EqualFold(
			filepath.Ext(entry.Name()),
			".docx",
		) {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)

	return files, nil
}

/* -------------------------------------------------------------------------- */
/* FILENAMES                                                                 */
/* -------------------------------------------------------------------------- */

func makeOutputName(
	name string,
	ext string,
) string {

	slug := slugify(name)

	return fmt.Sprintf(
		"%s%s",
		slug,
		ext,
	)
}

func slugify(value string) string {
	value = strings.TrimSpace(
		strings.ToLower(value),
	)

	// Preserve Unicode letters while replacing punctuation/spaces.
	var builder strings.Builder

	lastDash := false

	for _, r := range value {

		if isLetterOrNumber(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}

		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}

	result := strings.Trim(
		builder.String(),
		"-",
	)

	if result == "" {
		return "chapter"
	}

	return result
}

func isLetterOrNumber(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		(r >= 0x80)
}

/* -------------------------------------------------------------------------- */
/* SORTING                                                                   */
/* -------------------------------------------------------------------------- */

func sortDocuments(
	documents []DriveDocument,
) {

	sort.SliceStable(
		documents,
		func(i, j int) bool {

			ni := extractNumber(documents[i].Name)
			nj := extractNumber(documents[j].Name)

			if ni != nj {
				return ni < nj
			}

			return strings.ToLower(
				documents[i].Name,
			) < strings.ToLower(
				documents[j].Name,
			)
		},
	)
}

func extractNumber(value string) int {
	match := reDigits.FindString(value)

	if match == "" {
		return 999999
	}

	number, err := strconv.Atoi(match)

	if err != nil {
		return 999999
	}

	return number
}

/* -------------------------------------------------------------------------- */
/* DRIVE URL                                                                 */
/* -------------------------------------------------------------------------- */

func extractFolderID(input string) (string, error) {
	input = strings.TrimSpace(input)

	if input == "" {
		return "", errors.New(
			"folder ID or URL cannot be empty",
		)
	}

	// Plain folder ID.
	if !strings.Contains(input, "/") {
		return input, nil
	}

	parsed, err := url.Parse(input)

	if err != nil {
		return "", fmt.Errorf(
			"invalid Drive URL: %w",
			err,
		)
	}

	parts := strings.Split(
		strings.Trim(parsed.Path, "/"),
		"/",
	)

	for i, part := range parts {

		if part == "folders" && i+1 < len(parts) {
			id := parts[i+1]

			if id != "" {
				return id, nil
			}
		}
	}

	return "", errors.New(
		"could not find a Drive folder ID in the URL",
	)
}

func escapeDriveQuery(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

/* -------------------------------------------------------------------------- */
/* OAUTH                                                                     */
/* -------------------------------------------------------------------------- */

func createGoogleClient(
	ctx context.Context,
) (*http.Client, error) {

	credentialsPath := appFile(credentialsFile)

	credentials, err := os.ReadFile(
		credentialsPath,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"cannot read %s: %w",
			credentialsPath,
			err,
		)
	}

	config, err := google.ConfigFromJSON(
		credentials,
		Scope,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"invalid Google OAuth credentials: %w",
			err,
		)
	}

	token, err := loadToken()

	if err == nil {
		return config.Client(ctx, token), nil
	}

	fmt.Println("No Google authentication token found.")
	fmt.Println("Opening Google authorization...")
	fmt.Println()

	token, err = authenticateBrowser(
		ctx,
		config,
	)

	if err != nil {
		return nil, err
	}

	if err := saveToken(token); err != nil {
		return nil, err
	}

	return config.Client(ctx, token), nil
}

func loadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(appFile(tokenFile))

	if err != nil {
		return nil, err
	}

	var token oauth2.Token

	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

func saveToken(token *oauth2.Token) error {
	data, err := json.MarshalIndent(
		token,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	return os.WriteFile(
		appFile(tokenFile),
		append(data, '\n'),
		0600,
	)
}

func authenticateBrowser(
	ctx context.Context,
	config *oauth2.Config,
) (*oauth2.Token, error) {

	// Find an available local port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()

	redirectURL := "http://" + listener.Addr().String() + "/oauth2callback"
	config.RedirectURL = redirectURL

	authURL := config.AuthCodeURL(
		"gdocsync",
		oauth2.AccessTypeOffline,
	)

	authCode := make(chan string, 1)

	// Declare server first, then attach the handler, so the
	// closure can capture `server`.
	server := &http.Server{}

	server.Handler = http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth2callback" {
				http.NotFound(w, r)
				return
			}

			code := r.URL.Query().Get("code")
			if code == "" {
				fmt.Fprintln(w, "Authentication failed. You can close this window.")
				return
			}

			fmt.Fprintln(w, "Authentication successful. You can close this window.")

			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = server.Shutdown(context.Background())
			}()

			authCode <- code
		},
	)

	go func() {
		_ = server.Serve(listener)
	}()

	fmt.Println("Open this URL in your browser:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()

	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser automatically.")
	}

	select {
	case code := <-authCode:
		token, err := config.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf(
				"failed exchanging authorization code: %w",
				err,
			)
		}
		return token, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

/* -------------------------------------------------------------------------- */
/* BROWSER                                                                   */
/* -------------------------------------------------------------------------- */

func openBrowser(url string) error {

	var command string
	var args []string

	switch runtime.GOOS {

	case "windows":
		command = "rundll32"
		args = []string{
			"url.dll,FileProtocolHandler",
			url,
		}

	case "darwin":
		command = "open"
		args = []string{
			url,
		}

	default:
		command = "xdg-open"
		args = []string{
			url,
		}
	}

	return exec.Command(
		command,
		args...,
	).Start()
}

/* -------------------------------------------------------------------------- */
/* LOGOUT                                                                    */
/* -------------------------------------------------------------------------- */

func cmdLogout() {
	path := appFile(tokenFile)

	if err := os.Remove(path); err != nil {

		if os.IsNotExist(err) {
			fmt.Println("Already logged out.")
			return
		}

		log.Fatal(err)
	}

	fmt.Printf("Local Google OAuth token removed (%s).\n", path)
}