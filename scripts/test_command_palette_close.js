const fs = require('fs');
const assert = require('assert');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// Check that command-palette-modal contains a close button calling closeCommandPalette()
const paletteMatch = html.match(/id="command-palette-modal"[\s\S]*?<\/div>\s*<\/div>/);
assert(paletteMatch, 'command-palette-modal not found');

const paletteHtml = paletteMatch[0];

const hasCloseButton = /<button[^>]*onclick="closeCommandPalette\(\)"[^>]*>.*?✕.*?<\/button>/s.test(paletteHtml);
assert(hasCloseButton, 'Command palette modal should have a close button (✕) calling closeCommandPalette() like schema-search-modal');

console.log('PASS: Command palette modal has close button!');
