import { GoogleGenerativeAI } from '@google/generative-ai';
import fs from 'fs';
import path from 'path';
import { readEnvFile } from '../src/env.js';
import { GROUPS_DIR } from '../src/config.js';
import { logger } from '../src/logger.js';

async function run() {
  const secrets = readEnvFile([
    'GOOGLE_AI_API_KEY',
    'OBSIDIAN_API_KEY',
    'OBSIDIAN_PORT',
    'OBSIDIAN_HOST',
  ]);
  const apiKey = process.env.GOOGLE_AI_API_KEY || secrets.GOOGLE_AI_API_KEY;

  if (!apiKey) {
    logger.error('GOOGLE_AI_API_KEY not found in .env or environment');
    process.exit(1);
  }

  const obsidianKey = process.env.OBSIDIAN_API_KEY || secrets.OBSIDIAN_API_KEY;
  const obsidianPort = process.env.OBSIDIAN_PORT || secrets.OBSIDIAN_PORT || '27124';
  const obsidianHost = process.env.OBSIDIAN_HOST || secrets.OBSIDIAN_HOST || 'localhost';

  const genAI = new GoogleGenerativeAI(apiKey);
  const model = genAI.getGenerativeModel({
    model: 'gemini-flash-lite-latest',
    tools: [
      {
        functionDeclarations: [
          {
            name: 'write_research_file',
            description: 'Write research results to Obsidian (preferred) or a local file',
            parameters: {
              type: 'object',
              properties: {
                filename: { type: 'string', description: 'Name of the file to write (e.g. summary.md)' },
                content: { type: 'string', description: 'Content to write to the file' },
                vaultPath: { type: 'string', description: 'Optional vault path (e.g. Research/Topic.md)' },
              },
              required: ['filename', 'content'],
            },
          },
          {
            name: 'list_research_files',
            description: 'List all files in the main research directory',
            parameters: { type: 'object', properties: {} },
          },
        ],
      },
    ],
  });

  const researchDir = path.join(GROUPS_DIR, 'main', 'research');
  fs.mkdirSync(researchDir, { recursive: true });

  const prompt = process.argv.slice(2).join(' ') || 'Hello! What research can I help you with today?';
  logger.info({ prompt }, 'Starting Gemini research');

  const chat = model.startChat();
  let result = await chat.sendMessage(prompt);
  
  while (result.response.candidates?.[0]?.content?.parts?.some(p => p.functionCall)) {
    const calls = result.response.candidates[0].content.parts.filter(p => p.functionCall);
    const responses = [];

    for (const call of calls) {
      if (!call.functionCall) continue;
      const { name, args } = call.functionCall;
      logger.info({ name, args }, 'Calling tool');

      if (name === 'write_research_file') {
        const { filename, content, vaultPath } = args as { filename: string, content: string, vaultPath?: string };
        
        let status = 'success';
        let targetPath = '';

        if (obsidianKey) {
          const pathInVault = vaultPath || `Auto-Research/research/${filename}`;
          const url = `http://${obsidianHost}:${obsidianPort}/vault/${pathInVault}`;
          
          try {
            logger.info({ url }, 'Sending to Obsidian');
            const response = await fetch(url, {
              method: 'POST',
              headers: {
                'Authorization': `Bearer ${obsidianKey}`,
                'Content-Type': 'text/markdown',
              },
              body: content,
            });

            if (response.ok) {
              status = 'synced-to-obsidian';
              targetPath = pathInVault;
            } else {
              const errorText = await response.text();
              logger.warn({ status: response.status, errorText }, 'Obsidian sync failed, falling back to local');
              throw new Error(`Obsidian API error: ${response.status}`);
            }
          } catch (err) {
            const filePath = path.join(researchDir, filename);
            fs.writeFileSync(filePath, content);
            status = 'fallback-to-local';
            targetPath = filePath;
          }
        } else {
          const filePath = path.join(researchDir, filename);
          fs.writeFileSync(filePath, content);
          status = 'local-only';
          targetPath = filePath;
        }

        responses.push({
          functionResponse: {
            name: 'write_research_file',
            response: { status, path: targetPath },
          },
        });
      } else if (name === 'list_research_files') {
        const files = fs.readdirSync(researchDir);
        responses.push({
          functionResponse: {
            name: 'list_research_files',
            response: { files },
          },
        });
      }
    }

    result = await chat.sendMessage(responses);
  }

  console.log('\n--- Gemini Response ---\n');
  console.log(result.response.text());
  console.log('\n-----------------------\n');
}

run().catch(err => {
  logger.error({ err }, 'Research script failed');
  process.exit(1);
});
