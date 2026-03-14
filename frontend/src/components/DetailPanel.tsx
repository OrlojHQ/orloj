import { X } from "lucide-react";
import clsx from "clsx";
import { useState, type ReactNode } from "react";

interface Tab {
  id: string;
  label: string;
  content: ReactNode;
}

interface DetailPanelProps {
  title: string;
  subtitle?: string;
  tabs: Tab[];
  onClose: () => void;
  badge?: ReactNode;
}

export function DetailPanel({ title, subtitle, tabs, onClose, badge }: DetailPanelProps) {
  const [activeTab, setActiveTab] = useState(tabs[0]?.id ?? "");

  return (
    <div className="detail-panel">
      <div className="detail-panel__header">
        <div className="detail-panel__title-row">
          <div>
            <h2 className="detail-panel__title">{title}</h2>
            {subtitle && <p className="detail-panel__subtitle">{subtitle}</p>}
          </div>
          <div className="detail-panel__header-right">
            {badge}
            <button className="detail-panel__close" onClick={onClose} aria-label="Close">
              <X size={18} />
            </button>
          </div>
        </div>
        <div className="detail-panel__tabs">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              className={clsx("detail-panel__tab", activeTab === tab.id && "detail-panel__tab--active")}
              onClick={() => setActiveTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>
      <div className="detail-panel__body">
        {tabs.find((t) => t.id === activeTab)?.content}
      </div>
    </div>
  );
}
