import {
  DecisionGraph,
  JdmConfigProvider,
  useWasmReady,
} from "@gorules/jdm-editor";
import "./monaco";
import type { DecisionGraphType } from "@gorules/jdm-editor";
import "@gorules/jdm-editor/dist/style.css";
import type { JSONObject } from "../../lib/client";
import type { ResultRow } from "../analysis/model";
import type { Simulation } from "@gorules/jdm-editor";

export default function RawEditor({
  value,
  onChange,
  readOnly = false,
  traceRow,
}: {
  value: JSONObject;
  onChange: (value: JSONObject) => void;
  readOnly?: boolean;
  traceRow?: ResultRow | undefined;
}) {
  const ready = useWasmReady();
  return (
    <div className="graph-editor" data-editor-ready={ready}>
      <JdmConfigProvider>
        <DecisionGraph
          value={value as unknown as DecisionGraphType}
          components={[]}
          disabled={readOnly}
          hideLeftToolbar
          simulate={
            traceRow
              ? ((traceRow.error
                  ? { error: { message: traceRow.error, data: {} } }
                  : {
                      result: {
                        performance: "",
                        result: traceRow.output,
                        snapshot: value,
                        trace: traceRow.trace ?? {},
                      },
                    }) as Simulation)
              : undefined
          }
          onReactFlowInit={(flow) => {
            setTimeout(() => void flow.fitView({ padding: 0.2 }), 0);
          }}
          onChange={(graph) => onChange(graph as unknown as JSONObject)}
        />
      </JdmConfigProvider>
    </div>
  );
}
