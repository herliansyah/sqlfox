import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { renderFullMarkdownHtml, renderMarkdownBlockHtml } from './markdown';

describe('markdown renderer security & formatting', () => {
  it('blocks raw HTML and javascript: XSS payloads', () => {
    const html = renderFullMarkdownHtml('<script>alert(1)</script> [click](javascript:alert(1)) <img src=x onerror=alert(1)>');
    assert.ok(!html.includes('<script>'));
    assert.ok(!html.includes('<a '));
    assert.ok(!html.includes('href="javascript:'));
    assert.ok(!html.includes('<img '));
    assert.ok(html.includes('&lt;script&gt;'));
  });

  it('prevents NUL-byte sentinel amplification attacks', () => {
    const payload = '`secret_token` ' + '\u0000MD0\u0000'.repeat(10);
    const html = renderMarkdownBlockHtml(payload);
    const occurrences = html.split('secret_token').length - 1;
    assert.equal(occurrences, 1);
  });

  it('resolves nested inline code inside link labels without leaving raw sentinels', () => {
    const html = renderMarkdownBlockHtml('See [`user_id`](https://example.com/docs)');
    assert.ok(!html.includes('\u0000MD'));
    assert.ok(html.includes('<a href="https://example.com/docs"'));
    assert.ok(html.includes('<code'));
  });

  it('preserves snake_case SQL identifiers without italicizing underscores', () => {
    const html = renderMarkdownBlockHtml('Filter by user_id and created_at in orders_table');
    assert.ok(!html.includes('<em>'));
    assert.ok(html.includes('user_id'));
    assert.ok(html.includes('created_at'));
  });

  it('renders genuine _italic_ phrases when surrounded by boundaries', () => {
    const html = renderMarkdownBlockHtml('This is _important_ text.');
    assert.ok(html.includes('<em class="italic text-slate-300">important</em>'));
  });

  it('handles repeated unclosed code fences without hanging', () => {
    const unclosed = '```sql\n'.repeat(500) + 'SELECT 1;';
    const html = renderFullMarkdownHtml(unclosed);
    assert.ok(html.includes('SELECT 1;'));
  });
});
