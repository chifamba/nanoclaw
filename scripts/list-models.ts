import { GoogleGenerativeAI } from '@google/generative-ai';
import { readEnvFile } from '../src/env.js';

async function list() {
  const secrets = readEnvFile(['GOOGLE_AI_API_KEY']);
  const apiKey = process.env.GOOGLE_AI_API_KEY || secrets.GOOGLE_AI_API_KEY;
  if (!apiKey) {
    console.error('API key not found');
    return;
  }
  const genAI = new GoogleGenerativeAI(apiKey);
  const models = await genAI.listModels();
  console.log(models.models.map(m => m.name));
}

list();
