import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

/** GitHub-flavored Markdown renderer; links open in a new tab. */
export function MarkdownView({ source }: { source: string }) {
  return (
    <div className="prose-doc max-w-none text-sm leading-relaxed">
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
        }}
      >
        {source}
      </Markdown>
    </div>
  );
}
