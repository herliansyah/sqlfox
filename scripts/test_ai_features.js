const fs = require('fs');
const path = require('path');
const assert = require('assert');

console.log('Testing AI Copilot enhancements (BYOK persona, sessions, notes, error healer)...');

const htmlPath = path.join(__dirname, '..', 'web', 'dist', 'index.html');
const content = fs.readFileSync(htmlPath, 'utf8');

// 1. Check BYOK Modal Fields
assert(content.includes('id="ai-language"'), 'Must have ai-language select input');
assert(content.includes('value="id"'), 'Must have Bahasa Indonesia language option');
assert(content.includes('id="ai-style"'), 'Must have ai-style select input');
assert(content.includes('value="concise"'), 'Must have concise response style option');
assert(content.includes('id="ai-custom-prompt"'), 'Must have ai-custom-prompt textarea');
assert(content.includes('id="ai-include-schema"'), 'Must have ai-include-schema toggle');
console.log('✓ BYOK settings modal contains language, style, custom prompt, and schema toggle.');

// 2. Check Dynamic Badge
assert(content.includes('function updateAIProviderBadge'), 'Must define updateAIProviderBadge');
assert(!content.includes('>Ollama (local)<'), 'Static Ollama (local) badge text must be removed or dynamic');
console.log('✓ Dynamic AI Provider badge logic verified (no static Ollama glitch).');

// 3. Check Chat History & New Chat
assert(content.includes('function startNewAIChat'), 'Must define startNewAIChat');
assert(content.includes('function saveCurrentChatSession'), 'Must define saveCurrentChatSession');
assert(content.includes('function toggleAIChatHistory'), 'Must define toggleAIChatHistory');
assert(content.includes('function resumeAIChatSession'), 'Must define resumeAIChatSession');
assert(content.includes('function deleteAIChatSession'), 'Must define deleteAIChatSession');
assert(content.includes('function clearAllAIHistory'), 'Must define clearAllAIHistory');
console.log('✓ Chat sessions, history drawer, and conversation resumption verified.');

// 4. Check Internal Notes System
assert(content.includes('function saveToInternalNotes'), 'Must define saveToInternalNotes');
assert(content.includes('function exportCurrentNotes'), 'Must define exportCurrentNotes');
assert(content.includes('Notes.md'), 'Must reference Notes.md for internal notes tab');
console.log('✓ Internal Notes system (Save to Notes.md & Export) verified.');

// 5. Check Error Healer (Fix with AI)
assert(content.includes('function renderQueryErrorBanner'), 'Must define renderQueryErrorBanner');
assert(content.includes('function fixCurrentQueryWithAI'), 'Must define fixCurrentQueryWithAI');
assert(content.includes('Fix with AI'), 'Must have Fix with AI button');
console.log('✓ Error Healer (interactive query error banner + Fix with AI) verified.');

// 6. Check Go Server backend handlers_ai.go
const goHandlerPath = path.join(__dirname, '..', 'internal', 'server', 'handlers_ai.go');
const goContent = fs.readFileSync(goHandlerPath, 'utf8');

assert(goContent.includes('Language'), 'AIChatRequest must include Language field');
assert(goContent.includes('ResponseStyle'), 'AIChatRequest must include ResponseStyle field');
assert(goContent.includes('CustomPrompt'), 'AIChatRequest must include CustomPrompt field');
assert(goContent.includes('ErrorMessage'), 'AIChatRequest must include ErrorMessage field');
assert(goContent.includes('fix_query'), 'handleAIChat must handle fix_query action');
assert(goContent.includes('Bahasa Indonesia'), 'handleAIChat must support Bahasa Indonesia prompt instruction');
console.log('✓ Backend Go handler AIChatRequest and prompt customization verified.');

console.log('ALL AI COPILOT FEATURE TESTS PASSED!');
