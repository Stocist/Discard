import { marked } from 'marked';
import DOMPurify from 'dompurify';

marked.setOptions({
	gfm: true,
	breaks: true,
});

const renderer = new marked.Renderer();

// Links open in new tab
renderer.link = ({ href, text }) => {
	return `<a href="${href}" target="_blank" rel="noopener noreferrer">${text}</a>`;
};

marked.use({ renderer });

export function renderMarkdown(content: string): string {
	const raw = marked.parse(content, { async: false }) as string;
	return DOMPurify.sanitize(raw, {
		ALLOWED_TAGS: [
			'p', 'br', 'strong', 'em', 'del', 'code', 'pre',
			'a', 'ul', 'ol', 'li', 'blockquote', 'span',
		],
		ALLOWED_ATTR: ['href', 'target', 'rel', 'class'],
	});
}
