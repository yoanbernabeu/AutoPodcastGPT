package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OpenAI API endpoints and models configuration
const (
	OpenAITTSEndpoint = "https://api.openai.com/v1/audio/speech"
	OpenAIModelTTS    = "tts-1-hd" // changed to "tts-1-hd"
)

// Available options for podcast generation
var (
	availableLanguages = []string{
		"Afrikaans", "Arabic", "Armenian", "Azerbaijani", "Belarusian", "Bosnian",
		"Bulgarian", "Catalan", "Chinese", "Croatian", "Czech", "Danish", "Dutch",
		"English", "Estonian", "Finnish", "French", "Galician", "German", "Greek",
		"Hebrew", "Hindi", "Hungarian", "Icelandic", "Indonesian", "Italian",
		"Japanese", "Kannada", "Kazakh", "Korean", "Latvian", "Lithuanian",
		"Macedonian", "Malay", "Marathi", "Maori", "Nepali", "Norwegian", "Persian",
		"Polish", "Portuguese", "Romanian", "Russian", "Serbian", "Slovak",
		"Slovenian", "Spanish", "Swahili", "Swedish", "Tagalog", "Tamil", "Thai",
		"Turkish", "Ukrainian", "Urdu", "Vietnamese", "Welsh",
	}
	availableVoices = []string{
		"alloy", "ash", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer",
	}
)

// PromptData holds all the user input for the podcast
type PromptData struct {
	TextFile string
	Voice    string
	Language string
}

// Content represents a message content part
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Message represents a chat message structure
type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

// Custom error types for better error handling
type PodcastError struct {
	Stage   string
	Message string
	Err     error
}

// Error returns a formatted error message
func (e *PodcastError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("Error during %s: %s\nDetails: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("Error during %s: %s", e.Stage, e.Message)
}

// checkAPIKey verifies the presence and validity of the OpenAI API key
func checkAPIKey() error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return &PodcastError{
			Stage:   "initialization",
			Message: "OPENAI_API_KEY environment variable not set",
		}
	}

	// Test simple API call
	req, err := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	if err != nil {
		return &PodcastError{
			Stage:   "API validation",
			Message: "Failed to create test request",
			Err:     err,
		}
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return &PodcastError{
			Stage:   "API validation",
			Message: "Failed to connect to OpenAI API",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &PodcastError{
			Stage:   "API validation",
			Message: fmt.Sprintf("Invalid API key (status: %d)", resp.StatusCode),
		}
	}

	return nil
}

func main() {
	fmt.Println("=== Text-to-Speech Generator ===")

	// Initial API key verification
	fmt.Println("Checking OpenAI API key...")
	if err := checkAPIKey(); err != nil {
		fmt.Printf("\n❌ %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ API key valid")

	// Vérifier si un fichier a été passé en argument
	var promptData PromptData
	if len(os.Args) > 1 {
		textFile := os.Args[1]
		// Vérifier si le fichier existe
		if _, err := os.Stat(textFile); os.IsNotExist(err) {
			fmt.Printf("❌ File not found: %s\n", textFile)
			os.Exit(1)
		}
		// Collecter uniquement les autres paramètres
		promptData = collectUserInputWithFile(textFile)
	} else {
		// Collecter tous les paramètres via l'interface interactive
		promptData = collectUserInput()
	}

	// 2. Recap the information
	fmt.Println("\nRecap:")
	fmt.Printf("  Text File : %s\n", promptData.TextFile)
	fmt.Printf("  Voice    : %s\n", promptData.Voice)
	fmt.Printf("  Language : %s\n", promptData.Language)

	// 3. Load text from file
	fmt.Println("\nLoading text from file...")
	content, err := os.ReadFile(promptData.TextFile)
	if err != nil {
		fmt.Printf("\n❌ Error reading file: %v\n", err)
		os.Exit(1)
	}
	generatedText := string(content)
	fmt.Printf("✅ Text loaded (%d characters)\n", len(generatedText))

	// 4. Split the text into chunks (<= 2800 chars)
	fmt.Println("\nSplitting text into chunks ...")
	chunks := splitTextIntoChunks(generatedText, 2800)
	fmt.Printf("Created %d chunk(s).\n", len(chunks))

	// 5. Create a temporary directory to store partial audio files
	tmpDir, err := os.MkdirTemp(".", "podcast_tmp_")
	if err != nil {
		fmt.Printf("Error creating temp directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// 6. Générer l'audio pour tous les chunks en parallèle
	fmt.Println("\nGenerating audio for all chunks in parallel...")
	audioFiles, err := generateTTSAudioParallel(chunks, promptData.Voice, tmpDir)
	if err != nil {
		fmt.Printf("\n❌ Error during audio generation: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✅ Audio generation complete.")

	// 7. Concatenate all partial MP3 files
	finalOutput := "podcast_final.mp3"
	fmt.Printf("\nAssembling chunks into final file: %s\n", finalOutput)
	err = concatenateMP3Files(audioFiles, finalOutput)
	if err != nil {
		fmt.Printf("Error assembling final MP3: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Audio assembly complete!")
	fmt.Printf("Your final audio is saved as '%s'\n", finalOutput)
}

// getValidChoice displays options and gets a valid choice from user
func getValidChoice(reader *bufio.Reader, options []string, prompt string) string {
	for {
		fmt.Printf("\nAvailable %s:\n", prompt)
		for i, opt := range options {
			fmt.Printf("%d. %s\n", i+1, opt)
		}
		fmt.Print("Enter your choice (number): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if idx, err := strconv.Atoi(input); err == nil && idx > 0 && idx <= len(options) {
			return options[idx-1]
		}
		fmt.Println("Invalid choice. Please try again.")
	}
}

// collectUserInput asks the user (via console) for the required inputs
func collectUserInput() PromptData {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter the path to your text file: ")
	textFile, _ := reader.ReadString('\n')
	textFile = strings.TrimSpace(textFile)

	// Vérifier si le fichier existe
	if _, err := os.Stat(textFile); os.IsNotExist(err) {
		fmt.Printf("❌ File not found: %s\n", textFile)
		os.Exit(1)
	}

	voice := getValidChoice(reader, availableVoices, "voices")
	language := getValidChoice(reader, availableLanguages, "languages")

	return PromptData{
		TextFile: textFile,
		Voice:    voice,
		Language: language,
	}
}

// Nouvelle fonction pour collecter les entrées avec fichier prédéfini
func collectUserInputWithFile(textFile string) PromptData {
	reader := bufio.NewReader(os.Stdin)
	voice := getValidChoice(reader, availableVoices, "voices")
	language := getValidChoice(reader, availableLanguages, "languages")

	return PromptData{
		TextFile: textFile,
		Voice:    voice,
		Language: language,
	}
}

// Improved chunk splitting to avoid losing text
func splitTextIntoChunks(text string, maxSize int) []string {
	sentences := splitIntoSentences(text)
	var chunks []string
	var currentChunk strings.Builder

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// If this sentence alone exceeds maxSize, split it into smaller parts
		if len(sentence) > maxSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
			}
			// Split long sentence into smaller chunks while preserving words
			words := strings.Fields(sentence)
			var partialSentence strings.Builder
			for _, word := range words {
				if partialSentence.Len()+len(word)+1 > maxSize {
					if partialSentence.Len() > 0 {
						chunks = append(chunks, partialSentence.String())
						partialSentence.Reset()
					}
				}
				if partialSentence.Len() > 0 {
					partialSentence.WriteString(" ")
				}
				partialSentence.WriteString(word)
			}
			if partialSentence.Len() > 0 {
				chunks = append(chunks, partialSentence.String())
			}
			continue
		}

		// Normal sentence processing
		if currentChunk.Len()+len(sentence)+1 > maxSize {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
	}

	// Don't forget the last chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// splitIntoSentences splits text into sentences using basic punctuation rules
func splitIntoSentences(text string) []string {
	separators := []string{".", "?", "!"}
	var replacerStr = text

	for _, sep := range separators {
		replacerStr = strings.ReplaceAll(replacerStr, sep, sep+"|SEP|")
	}

	parts := strings.Split(replacerStr, "|SEP|")

	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// generateTTSAudio calls the TTS endpoint with given text chunk and
// saves the resulting MP3 to the provided outputFile path
func generateTTSAudio(textChunk, voice, outputFile string) error {
	if len(strings.TrimSpace(textChunk)) == 0 {
		return &PodcastError{
			Stage:   "audio generation",
			Message: "Empty text chunk",
		}
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return &PodcastError{
			Stage:   "audio generation",
			Message: "API key not found",
		}
	}

	payload := map[string]interface{}{
		"model": OpenAIModelTTS,
		"input": textChunk,
		"voice": voice,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", OpenAITTSEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("\n🔍 OpenAI API Response:\n%s\n", string(body))
		return &PodcastError{
			Stage:   "audio generation",
			Message: fmt.Sprintf("TTS request failed (status %d)", resp.StatusCode),
			Err:     fmt.Errorf(string(body)),
		}
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}

	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return err
	}

	// Verify audio file size after creation
	stat, err := outFile.Stat()
	if err != nil || stat.Size() == 0 {
		return &PodcastError{
			Stage:   "audio generation",
			Message: "Generated audio file is empty",
			Err:     err,
		}
	}

	return nil
}

// concatenateMP3Files combines multiple MP3 files into a single output file
func concatenateMP3Files(files []string, output string) error {
	outFile, err := os.Create(output)
	if err != nil {
		return err
	}

	defer outFile.Close()

	for _, f := range files {
		inFile, err := os.Open(f)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(outFile, inFile)
		inFile.Close()
		if copyErr != nil {
			return copyErr
		}
	}

	return nil
}

// showSpinner displays a loading animation during long operations
func showSpinner(done chan bool) {
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Printf("\r") // Clear the spinner
			return
		default:
			fmt.Printf("\r%s Processing...", spinner[i])
			i = (i + 1) % len(spinner)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// saveGeneratedText saves the generated text to a file
// stringToInt converts a string to an integer, returning 0 if conversion fails
func stringToInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

// countWords returns the number of words in a string
func countWords(s string) int {
	return len(strings.Fields(s))
}

func saveGeneratedText(text string) error {
	if len(strings.TrimSpace(text)) == 0 {
		return &PodcastError{
			Stage:   "text saving",
			Message: "Empty text content",
		}
	}

	err := os.WriteFile("text.txt", []byte(text), 0644)
	if err != nil {
		return &PodcastError{
			Stage:   "text saving",
			Message: "Failed to write file",
			Err:     err,
		}
	}

	// Verify the file was written correctly
	content, err := os.ReadFile("text.txt")
	if err != nil || len(content) == 0 {
		return &PodcastError{
			Stage:   "text saving",
			Message: "File verification failed",
			Err:     err,
		}
	}

	return nil
}

// generateTTSAudioParallel gère la génération audio en parallèle
func generateTTSAudioParallel(chunks []string, voice string, tmpDir string) ([]string, error) {
	audioFiles := make([]string, len(chunks))
	errors := make(chan error, len(chunks))
	progress := make(chan int, len(chunks))
	var wg sync.WaitGroup

	// Limiter le nombre de goroutines concurrentes
	semaphore := make(chan struct{}, 5)

	// Démarrer la goroutine d'affichage de la progression
	progressDone := make(chan bool)
	go func() {
		completed := 0
		total := len(chunks)
		for range progress {
			completed++
			// Effacer la ligne précédente
			fmt.Printf("\r\033[K")
			// Afficher la barre de progression
			fmt.Printf("Progress: [")
			width := 30
			pos := width * completed / total
			for i := 0; i < width; i++ {
				if i < pos {
					fmt.Print("=")
				} else if i == pos {
					fmt.Print(">")
				} else {
					fmt.Print(" ")
				}
			}
			fmt.Printf("] %d/%d chunks complete", completed, total)
		}
		progressDone <- true
	}()

	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, text string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d.mp3", index+1))
			err := generateTTSAudio(text, voice, chunkFile)
			if err != nil {
				errors <- fmt.Errorf("chunk %d: %w", index+1, err)
				return
			}
			audioFiles[index] = chunkFile
			progress <- 1 // Signaler qu'un chunk est terminé
		}(i, chunk)
	}

	// Attendre que toutes les goroutines soient terminées
	wg.Wait()
	close(errors)
	close(progress)

	// Attendre que l'affichage de la progression soit terminé
	<-progressDone

	// Vérifier s'il y a eu des erreurs
	for err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return audioFiles, nil
}
