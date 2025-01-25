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
	"time"
)

// OpenAI API endpoints and models configuration
const (
	OpenAIChatEndpoint    = "https://api.openai.com/v1/chat/completions"
	OpenAITTSEndpoint     = "https://api.openai.com/v1/audio/speech"
	OpenAIModelGeneration = "o1-preview" // e.g. "o1-2024-12-17"
	OpenAIModelTTS        = "tts-1-hd"   // changed to "tts-1-hd"
	WPM                   = 178          // Words Per Minute for OpenAI voices
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
	availableDurations = []string{"5", "10", "15", "20", "30"}
)

// PromptData holds all the user input for the podcast
type PromptData struct {
	Prompt   string
	Style    string
	Duration string
	Voice    string
	Language string
	TextFile string // Nouveau champ pour le chemin du fichier
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
	fmt.Println("=== Podcast Generator CLI ===")

	// Initial API key verification
	fmt.Println("Checking OpenAI API key...")
	if err := checkAPIKey(); err != nil {
		fmt.Printf("\n❌ %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ API key valid")

	// 1. Collect user input
	promptData := collectUserInput()

	// 2. Recap the information
	fmt.Println("\nRecap:")
	fmt.Printf("  Prompt   : %s\n", promptData.Prompt)
	fmt.Printf("  Style    : %s\n", promptData.Style)
	fmt.Printf("  Duration : %s minutes\n", promptData.Duration)
	fmt.Printf("  Voice    : %s\n", promptData.Voice)
	fmt.Printf("  Language : %s\n", promptData.Language)

	// 3. Generate or load text
	var generatedText string
	if promptData.TextFile != "" {
		fmt.Println("\nLoading text from file...")
		content, err := os.ReadFile(promptData.TextFile)
		if err != nil {
			fmt.Printf("\n❌ Error reading file: %v\n", err)
			os.Exit(1)
		}
		generatedText = string(content)
		fmt.Printf("✅ Text loaded (%d characters)\n", len(generatedText))
	} else {
		fmt.Println("\nGenerating text from OpenAI ...")
		var done = make(chan bool)
		defer close(done)
		go showSpinner(done)
		var err error
		generatedText, err = generatePodcastText(promptData)
		done <- true
		if err != nil {
			if pe, ok := err.(*PodcastError); ok {
				fmt.Printf("\n❌ Error: %v\n", pe)
			} else {
				fmt.Printf("\n❌ Unexpected error: %v\n", err)
			}
			os.Exit(1)
		}
		fmt.Printf("✅ Text generation complete (%d characters)\n", len(generatedText))
	}

	// Save and verify text
	fmt.Println("\nSaving text to file...")
	err := saveGeneratedText(generatedText)
	if err != nil {
		fmt.Printf("\n❌ Error saving text: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Text saved and verified")

	// 4. Split the generated text into chunks (<= 2800 chars)
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

	// 6. For each chunk, call TTS to get audio, save to file
	fmt.Println("\nRequesting TTS for each chunk ...")
	audioFiles := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		fmt.Printf("\n  - Generating audio for chunk %d/%d\n", i+1, len(chunks))
		var done = make(chan bool)
		defer close(done)
		go showSpinner(done)
		chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d.mp3", i+1))
		err = generateTTSAudio(chunk, promptData.Voice, chunkFile)
		done <- true
		if err != nil {
			if pe, ok := err.(*PodcastError); ok {
				fmt.Printf("\n❌ Error: %v\n", pe)
			} else {
				fmt.Printf("\n❌ Error generating TTS for chunk %d: %v\n", i+1, err)
			}
			os.Exit(1)
		}
		audioFiles = append(audioFiles, chunkFile)
		time.Sleep(time.Second) // simple rate-limit or to give feedback
	}
	fmt.Println("TTS generation complete.")

	// 7. Concatenate all partial MP3 files
	finalOutput := "podcast_final.mp3"
	fmt.Printf("\nAssembling chunks into final file: %s\n", finalOutput)
	err = concatenateMP3Files(audioFiles, finalOutput)
	if err != nil {
		fmt.Printf("Error assembling final MP3: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Podcast generation complete!")
	fmt.Printf("Your final podcast is saved as '%s'\n", finalOutput)
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

	fmt.Println("\nDo you want to generate text or use an existing file? (G)enerate/(F)ile: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.ToUpper(strings.TrimSpace(choice))

	var prompt, style, textFile string

	if choice == "F" {
		fmt.Print("Enter the path to your text file: ")
		textFile, _ = reader.ReadString('\n')
		textFile = strings.TrimSpace(textFile)
		// Vérifier si le fichier existe
		if _, err := os.Stat(textFile); os.IsNotExist(err) {
			fmt.Printf("❌ File not found: %s\n", textFile)
			os.Exit(1)
		}
	} else {
		fmt.Print("Enter your podcast idea (Prompt): ")
		prompt, _ = reader.ReadString('\n')
		prompt = strings.TrimSpace(prompt)

		fmt.Println("\nSelect a style from the following options (Science fiction, Fantasy, Romance, etc.): ")
		fmt.Print("> ")
		style, _ = reader.ReadString('\n')
		style = strings.TrimSpace(style)
	}

	duration := getValidChoice(reader, availableDurations, "durations (in minutes)")
	voice := getValidChoice(reader, availableVoices, "voices")
	language := getValidChoice(reader, availableLanguages, "languages")

	return PromptData{
		Prompt:   prompt,
		Style:    style,
		Duration: duration,
		Voice:    voice,
		Language: language,
		TextFile: textFile,
	}
}

// Map des traductions de "FIN" dans différentes langues
var endWordByLanguage = map[string]string{
	"French":     "FIN",
	"English":    "THE END",
	"Spanish":    "FIN",
	"German":     "ENDE",
	"Italian":    "FINE",
	"Portuguese": "FIM",
	// Par défaut "THE END" pour les autres langues
}

func getEndWord(language string) string {
	if word, ok := endWordByLanguage[language]; ok {
		return word
	}
	return "THE END"
}

func generatePodcastText(p PromptData) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", &PodcastError{
			Stage:   "text generation",
			Message: "API key not found",
		}
	}

	// Fonction helper pour générer le texte
	generateText := func(extraDuration float64) (string, error) {
		adjustedDuration := float64(stringToInt(p.Duration)) + extraDuration
		targetWords := int(adjustedDuration * WPM)

		combinedPrompt := fmt.Sprintf(
			`You are tasked with writing a %s story in %s that will be exactly %.1f minutes long when read by an AI voice.

Story context: %s

IMPORTANT LENGTH REQUIREMENTS:
The story MUST be %d words long (based on AI voice reading speed of %d words per minute).
This is not a suggestion but a strict requirement for proper audio timing.

Writing guidelines:
- Write a complete story that naturally fills the entire duration
- Use rich, descriptive language that flows well when spoken
- Avoid dialogue marks or special formatting
- Create a clear narrative arc (beginning, middle, end)
- Make the pacing consistent throughout
- Focus on engaging, spoken-word storytelling
- The story MUST end with the word "%s" on a new line after a pause

Note: The word count is crucial for timing - aim for exactly %d words (including the ending word).`,
			p.Style, p.Language, adjustedDuration,
			p.Prompt,
			targetWords, WPM,
			getEndWord(p.Language),
			targetWords,
		)

		// Nouvelle structure pour le contenu du message
		type Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}

		// Nouvelle structure pour le message
		type Message struct {
			Role    string    `json:"role"`
			Content []Content `json:"content"`
		}

		payload := map[string]interface{}{
			"model": OpenAIModelGeneration,
			"messages": []Message{
				{
					Role: "user",
					Content: []Content{
						{
							Type: "text",
							Text: combinedPrompt,
						},
					},
				},
			},
			"max_completion_tokens": 4000,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}

		req, err := http.NewRequest("POST", OpenAIChatEndpoint, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", err
		}

		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("\n🔍 OpenAI API Response:\n%s\n", string(body))
			return "", &PodcastError{
				Stage:   "text generation",
				Message: fmt.Sprintf("API request failed (status %d)", resp.StatusCode),
				Err:     fmt.Errorf(string(body)),
			}
		}

		// Modified response structure to handle both string and array content
		var responseData struct {
			Choices []struct {
				Message struct {
					Content interface{} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"usage"`
		}

		err = json.NewDecoder(resp.Body).Decode(&responseData)
		if err != nil {
			return "", err
		}

		if len(responseData.Choices) == 0 {
			return "", fmt.Errorf("no text returned by the model")
		}

		// Handle both string and array content types
		var content string
		switch v := responseData.Choices[0].Message.Content.(type) {
		case string:
			content = v
		case []interface{}:
			// Handle array of content objects
			if len(v) > 0 {
				if contentObj, ok := v[0].(map[string]interface{}); ok {
					if text, ok := contentObj["text"].(string); ok {
						content = text
					}
				}
			}
		default:
			return "", fmt.Errorf("unexpected content format from API")
		}

		if content == "" {
			return "", fmt.Errorf("no text content in response")
		}

		actualWords := countWords(content)
		estimatedDuration := float64(actualWords) / WPM

		fmt.Printf("\nGeneration stats:\n")
		fmt.Printf("- Target words: %d\n", targetWords)
		fmt.Printf("- Actual words: %d\n", actualWords)
		fmt.Printf("- Target duration: %.1f minutes\n", adjustedDuration)
		fmt.Printf("- Estimated duration: %.1f minutes\n", estimatedDuration)
		fmt.Printf("- Total tokens: %d\n", responseData.Usage.TotalTokens)

		// Après avoir reçu le contenu, s'assurer qu'il se termine bien par le mot approprié
		endWord := getEndWord(p.Language)
		if !strings.HasSuffix(strings.TrimSpace(content), endWord) {
			content = strings.TrimSpace(content) + "\n\n" + endWord
		}

		return content, nil
	}

	// Première tentative
	content, err := generateText(0)
	if err != nil {
		return "", err
	}

	// Vérification de la durée
	actualWords := countWords(content)
	estimatedDuration := float64(actualWords) / WPM
	targetDuration := float64(stringToInt(p.Duration))

	// Si la durée est trop courte (moins de 90% de la durée cible)
	if estimatedDuration < targetDuration*0.9 {
		fmt.Printf("\n⚠️  Generated content is too short (%.1f vs %s minutes)\n", estimatedDuration, p.Duration)
		fmt.Println("🔄 Trying to extend the existing content...")

		// Deuxième tentative en demandant d'allonger le texte existant
		extensionPrompt := fmt.Sprintf(
			`Here is a story that needs to be longer. Currently it's %.1f minutes long, but it needs to be %s minutes.
Please extend this story while maintaining the same style and flow. Add more details, descriptions, and story elements.

Current story:
%s

Make the story longer while keeping its coherence and quality. Target duration: %s minutes.`,
			estimatedDuration, p.Duration, content, p.Duration,
		)

		payload := map[string]interface{}{
			"model": OpenAIModelGeneration,
			"messages": []Message{
				{
					Role: "user",
					Content: []Content{
						{
							Type: "text",
							Text: extensionPrompt,
						},
					},
				},
			},
			"max_completion_tokens": 4000,
		}

		// Appel à l'API pour étendre le texte
		jsonData, err := json.Marshal(payload)
		if err == nil {
			req, err := http.NewRequest("POST", OpenAIChatEndpoint, bytes.NewBuffer(jsonData))
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err == nil {
					defer resp.Body.Close()

					if resp.StatusCode == http.StatusOK {
						var responseData struct {
							Choices []struct {
								Message struct {
									Content []Content `json:"content"`
								} `json:"message"`
							} `json:"choices"`
						}

						if json.NewDecoder(resp.Body).Decode(&responseData) == nil &&
							len(responseData.Choices) > 0 &&
							len(responseData.Choices[0].Message.Content) > len(content) {
							// Utiliser le nouveau contenu seulement s'il est plus long
							content = responseData.Choices[0].Message.Content[0].Text
							fmt.Printf("✅ Successfully extended the content to %d words\n",
								countWords(content))
						}
					}
				}
			}
		}

		// Si quelque chose a échoué, on garde le contenu original
		if countWords(content) == actualWords {
			fmt.Println("⚠️  Extension failed, keeping original content")
		}
	}

	return content, nil
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
