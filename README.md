# AutoPodcastGPT 🎙️

Generate audio podcasts automatically using OpenAI's APIs for text generation and text-to-speech conversion.

## Features

- 🤖 Uses OpenAI's latest models for text generation
- 🗣️ High-quality text-to-speech conversion
- 🌍 Supports multiple languages
- 🎭 Various voice options
- ⏱️ Flexible duration settings
- 📝 Pure narrative text optimized for audio

## Prerequisites

- Go 1.20 or higher
- OpenAI API key

## Installation

```bash
git clone https://github.com/yourusername/AutoPodcastGPT.git
cd AutoPodcastGPT
go mod tidy
```

## Configuration

Set your OpenAI API key as an environment variable:

```bash
export OPENAI_API_KEY='your-api-key-here'
```

## Usage

```bash
go run main.go
```

Follow the interactive prompts to:
1. Enter your podcast idea
2. Choose a style
3. Select duration
4. Pick a voice
5. Choose language

## Available Options

### Voices
- alloy, ash, coral, echo, fable, onyx, nova, sage, shimmer

### Durations
- 5, 10, 15, 20, 30 minutes

### Languages
Multiple languages supported including French, English, Spanish, etc.

## Output

The program generates:
- A final MP3 file (`podcast_final.mp3`)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
