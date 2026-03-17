import { useState, useEffect } from "react";
import { Trash2, Save, Plus, X, AlertCircle, CheckCircle2, Bot, Cpu } from "lucide-react";

const LLMs = () => {
  const [llmConfigs, setLlmConfigs] = useState<any[]>([]);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Form states
  const [llmName, setLlmName] = useState("");
  const [llmProvider] = useState("gemini");
  const [llmModel, setLlmModel] = useState("gemini-2.0-flash");
  const [llmKey, setLlmKey] = useState("");

  const fetchLLMs = async () => {
    try {
      const response = await fetch("/api/llm-configs", {
        headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
      });
      if (response.ok) setLlmConfigs(await response.json());
    } catch (err) {
      console.error("Failed to fetch LLMs:", err);
    }
  };

  useEffect(() => {
    fetchLLMs();
  }, []);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    try {
      const response = await fetch("/api/llm-configs", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": `Bearer ${localStorage.getItem("token")}` },
        body: JSON.stringify({ name: llmName, provider: llmProvider, model: llmModel, api_key: llmKey }),
      });
      if (!response.ok) throw new Error("Failed to save LLM config");
      
      setMessage({ type: 'success', text: "AI Model saved!" });
      fetchLLMs();
      setTimeout(() => setIsDrawerOpen(false), 1500);
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm("Delete this configuration?")) return;
    await fetch(`/api/llm-configs/${id}`, {
      method: "DELETE",
      headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
    });
    fetchLLMs();
  };

  return (
    <div className="space-y-8 animate-in slide-in-from-bottom-4 duration-500">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">AI Models</h2>
          <p className="text-slate-500 mt-1">Configure your LLM providers for message summarization.</p>
        </div>
        <button 
          onClick={() => { setIsDrawerOpen(true); setMessage(null); }}
          className="bg-blue-600 hover:bg-blue-700 text-white px-6 py-3 rounded-2xl shadow-lg shadow-blue-200 flex items-center gap-2 font-bold transition-all active:scale-[0.98]"
        >
          <Plus className="w-5 h-5" />
          Add Model
        </button>
      </header>

      <div className="bg-white p-8 rounded-[40px] border border-slate-100 shadow-sm min-h-[400px]">
        {llmConfigs.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="bg-slate-50 p-6 rounded-[32px] mb-4">
              <Bot className="w-12 h-12 text-slate-200" />
            </div>
            <h3 className="text-lg font-bold text-slate-900">No AI Models configured</h3>
            <p className="text-slate-400 max-w-xs mt-1">Add a model like Gemini to enable message summarization.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {llmConfigs.map(item => (
              <div key={item.id} className="p-6 bg-slate-50/50 rounded-[32px] border border-slate-100 group hover:bg-white hover:shadow-md transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="bg-white p-3 rounded-2xl shadow-sm ring-1 ring-slate-100 text-blue-600">
                    <Cpu className="w-6 h-6" />
                  </div>
                  <button onClick={() => handleDelete(item.id)} className="p-2 text-slate-300 hover:text-rose-500 opacity-0 group-hover:opacity-100 transition-all">
                    <Trash2 className="w-5 h-5" />
                  </button>
                </div>
                <div>
                  <p className="font-black text-slate-900 capitalize text-lg">{item.name}</p>
                  <p className="text-[10px] text-slate-400 font-black uppercase tracking-[0.2em] mt-1">
                    {item.provider} / {item.model}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {isDrawerOpen && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm animate-in fade-in duration-300" onClick={() => setIsDrawerOpen(false)} />
          <div className="relative w-full max-w-md bg-white h-full shadow-2xl animate-in slide-in-from-right duration-300 flex flex-col">
            <div className="p-8 border-b border-slate-100 flex items-center justify-between bg-slate-50/50">
              <div>
                <h3 className="text-2xl font-black text-slate-900">Add AI Model</h3>
                <p className="text-sm text-slate-500 mt-1 font-medium">Configure a new LLM provider</p>
              </div>
              <button onClick={() => setIsDrawerOpen(false)} className="p-2 hover:bg-white rounded-2xl transition-colors text-slate-400"><X className="w-6 h-6" /></button>
            </div>

            <div className="flex-1 overflow-y-auto p-8 space-y-10">
              {message && (
                <div className={`p-4 rounded-2xl flex items-center gap-3 animate-in zoom-in duration-300 ${message.type === 'success' ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'}`}>
                  {message.type === 'success' ? <CheckCircle2 className="w-5 h-5" /> : <AlertCircle className="w-5 h-5" />}
                  <p className="text-sm font-bold">{message.text}</p>
                </div>
              )}

              <form onSubmit={handleSave} className="space-y-6">
                <div className="space-y-2">
                  <label className="text-sm font-bold text-slate-700 ml-1">Config Name</label>
                  <input type="text" value={llmName} onChange={e => setLlmName(e.target.value)} className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 outline-none" placeholder="My Gemini Pro" required />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-bold text-slate-700 ml-1">Provider</label>
                  <div className="flex items-center gap-3 p-4 rounded-3xl border-2 border-blue-600 bg-blue-50/50 text-blue-700">
                    <Bot className="w-5 h-5" />
                    <span className="font-bold text-sm">Google Gemini</span>
                  </div>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-bold text-slate-700 ml-1">Model Name</label>
                  <input type="text" value={llmModel} onChange={e => setLlmModel(e.target.value)} className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 outline-none" placeholder="gemini-2.0-flash" required />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-bold text-slate-700 ml-1">API Key</label>
                  <input type="password" value={llmKey} onChange={e => setLlmKey(e.target.value)} className="w-full px-5 py-4 bg-slate-50 border border-slate-200 rounded-3xl focus:ring-4 focus:ring-blue-100 outline-none" placeholder="AIza..." required />
                </div>

                <button type="submit" disabled={isLoading} className="w-full bg-slate-900 text-white font-black py-5 rounded-[32px] shadow-2xl flex items-center justify-center gap-3 active:scale-[0.98] transition-all disabled:opacity-70 mt-4">
                  {isLoading ? <div className="w-6 h-6 border-2 border-white/30 border-t-white rounded-full animate-spin" /> : <Save className="w-6 h-6" />}
                  Save AI Model
                </button>
              </form>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default LLMs;
