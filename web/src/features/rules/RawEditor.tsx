import { DecisionGraph, JdmConfigProvider } from "@gorules/jdm-editor";
import type { DecisionGraphType } from "@gorules/jdm-editor";
import "@gorules/jdm-editor/dist/style.css";
import type { JSONObject } from "../../lib/client";

export default function RawEditor({
  value,
  onChange,
}: {
  value: JSONObject;
  onChange: (value: JSONObject) => void;
}) {
  return (
    <div className="graph-editor">
      <JdmConfigProvider>
        <DecisionGraph
          value={value as unknown as DecisionGraphType}
          components={[]}
          hideLeftToolbar
          onReactFlowInit={(flow) => {
            setTimeout(() => void flow.fitView({ padding: 0.2 }), 0);
          }}
          onChange={(graph) => onChange(graph as unknown as JSONObject)}
        />
      </JdmConfigProvider>
    </div>
  );
}
