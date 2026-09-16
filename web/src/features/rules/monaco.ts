import * as monaco from "monaco-editor";
import { loader } from "@monaco-editor/react";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import TypeScriptWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";
import JSONWorker from "monaco-editor/esm/vs/language/json/json.worker?worker";

// Keep the function and schema editors usable in an offline deployment.
self.MonacoEnvironment = {
  getWorker(_moduleId, label) {
    if (label === "javascript" || label === "typescript")
      return new TypeScriptWorker();
    if (label === "json") return new JSONWorker();
    return new EditorWorker();
  },
};
loader.config({ monaco });
