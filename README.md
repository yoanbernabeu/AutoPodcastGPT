# Text-to-Speech Generator 🎙️

Convert text files into high-quality audio using OpenAI's text-to-speech API.

## Features

- 🗣️ High-quality text-to-speech conversion
- 🌍 Supports multiple languages
- 🎭 Various voice options
- 📄 Support for text files
- 🔄 Automatic text chunking for optimal processing

## Prerequisites

- Go 1.20 or higher
- OpenAI API key

## Installation

```bash
git clone https://github.com/yourusername/text-to-speech-generator.git
cd text-to-speech-generator
go mod tidy
```

## Configuration

Set your OpenAI API key as an environment variable:

```bash
export OPENAI_API_KEY='your-api-key-here'
```

## Usage

There are two ways to use the program:

### 1. Interactive Mode
```bash
go run main.go
```

### 2. File Argument Mode
```bash
go run main.go path/to/your/text/file.txt
```

In both cases, you will then need to select:
1. A voice
2. A language

## Text File Requirements
- Plain text format (.txt recommended)
- Content should be narrative-style text
- Avoid special formatting or dialogue marks

## Available Options

### Voices
- alloy, ash, coral, echo, fable, onyx, nova, sage, shimmer

### Languages
Multiple languages supported including:
- French
- English
- Spanish
- German
- Italian
- And many more...

## Output

The program generates an MP3 file (`podcast_final.mp3`) from your input text.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
