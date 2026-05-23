import type { AgentCard } from "../api/types";

interface AgentCardPreviewProps {
  card: AgentCard;
}

export function AgentCardPreview({ card }: AgentCardPreviewProps) {
  return (
    <div className="card">
      <h3 className="card__title" style={{ marginBottom: "0.5rem" }}>Agent Card</h3>
      <pre className="detail-field__pre" style={{ maxHeight: "400px", overflow: "auto" }}>
        {JSON.stringify(card, null, 2)}
      </pre>
    </div>
  );
}
