# Discord Channel Merger Bot

This Go package provides a Discord bot that reads messages from specified Discord channels, stores them in order of when the messages were delivered in PebbleDB, and then sends them to a specified Discord channel. It includes functionality to handle message content, message pins and attachments, and ensures proper error handling and resource cleanup.

## Features

- Reads messages from specified Discord channels.
- Stores messages in order PebbleDB.
- Sends stored messages to a specified Discord channel.
- Handles message content and attachments.
- Ensures proper error handling and resource cleanup.

## Installation

1. Clone the repository:

   ```sh
   git clone https://github.com/yourusername/discord-ch-merge.git
   cd discord-ch-merge
   ```

2. Build:

   ```sh
    go build -o discord-ch-merge .
   ```

## Configuration

The bot requires the following configuration parameters:

- `FROM`: A list of source channel IDs to read messages from.
- `TO`: The destination channel ID to send messages to.
- `TOKEN`: The Discord bot token.

You can set these parameters using environment variables or command-line flags.

## Usage

Run the bot with the required configuration:

```sh
./discord-ch-merge  --from <source_channel_id> --to <destination_channel_id> --token <bot_token>
```

Example:

```sh
./discord-ch-merge --from 123456789012345678 --to 987654321098765432 --token YOUR_BOT_TOKEN
```

## License

This project is licensed under the MIT License.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Acknowledgements

- [discordgo](https://github.com/bwmarrin/discordgo) - Discord API wrapper for Go.
- [Pebble](https://github.com/cockroachdb/pebble) - Key-value store library for Go.
- [zap](https://github.com/uber-go/zap) - Fast, structured, leveled logging in Go.
