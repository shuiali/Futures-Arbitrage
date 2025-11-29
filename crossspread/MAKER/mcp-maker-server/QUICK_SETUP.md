# Quick Setup Guide for MAKER MCP Server

## Add to VS Code Settings

Copy this configuration to your VS Code settings:

### Option 1: User Settings (Recommended)

Press `Ctrl+Shift+P` → "Preferences: Open User Settings (JSON)" and add:

```json
{
  "mcp": {
    "servers": {
      "maker": {
        "command": "node",
        "args": ["e:/Futures Arbitrage/MAKER/mcp-maker-server/dist/index.js"]
      }
    }
  }
}
```

### Option 2: Workspace Settings

The `.vscode/mcp.json` file is already configured for this workspace.

## Restart VS Code

After adding the configuration, restart VS Code completely.

## Verify Installation

In Copilot Chat, type:
```
@maker What tools do you have available?
```

You should see the MAKER tools listed.

## Quick Reference - Available Commands

| Command | Purpose |
|---------|---------|
| `@maker analyze [task]` | Get MAKER recommendations for a task |
| `@maker decompose [task]` | Break task into atomic subtasks |
| `@maker validate [code]` | Check code quality with MAKER principles |
| `@maker check [response]` | Detect red flags in a response |

## Example Usage

```
@maker I need to implement a REST API endpoint for user authentication.
Please use the full MAKER workflow to ensure high quality.
```
