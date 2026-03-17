import { useState, useEffect } from "react";
import { Plus, Trash2, Layers, AlertCircle, CheckCircle2, Clock, ArrowRight, Bot } from "lucide-react";

const Subscriptions = () => {
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);
  const [subscriptions, setSubscriptions] = useState<any[]>([]);
  const [credentials, setCredentials] = useState<any[]>([]);
  const [llmConfigs, setLlmConfigs] = useState<any[]>([]);
  const [showAddSub, setShowAddSub] = useState(false);

  // New Subscription Form states
  const [selectedSource, setSelectedSource] = useState("");
  const [selectedDest, setSelectedDest] = useState("");
  const [enableSummarization, setEnableSummarization] = useState(true);
  const [selectedLLM, setSelectedLLM] = useState("");
  const [syncInterval, setSyncInterval] = useState(300);

  const fetchConfig = async () => {
    try {
      const headers = { "Authorization": `Bearer ${localStorage.getItem("token")}` };
      const [credsRes, subsRes, llmRes] = await Promise.all([
        fetch("/api/credentials", { headers }),
        fetch("/api/subscriptions", { headers }),
        fetch("/api/llm-configs", { headers })
      ]);
      
      if (credsRes.ok) setCredentials(await credsRes.json());
      if (subsRes.ok) setSubscriptions(await subsRes.json());
      if (llmRes.ok) setLlmConfigs(await llmRes.json());
    } catch (err) {
      console.error("Failed to fetch config:", err);
    }
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const handleCreateSub = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    try {
      const response = await fetch("/api/subscriptions", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": `Bearer ${localStorage.getItem("token")}`
        },
        body: JSON.stringify({
          source_credential_id: parseInt(selectedSource),
          destination_credential_id: parseInt(selectedDest),
          enable_summarization: enableSummarization,
          llm_config_id: enableSummarization && selectedLLM ? parseInt(selectedLLM) : null,
          sync_interval: syncInterval
        }),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || "Failed to create subscription");
      }

      setShowAddSub(false);
      setSelectedSource("");
      setSelectedDest("");
      setSelectedLLM("");
      fetchConfig();
      setMessage({ type: 'success', text: "Aggregation task created successfully!" });
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteSub = async (id: number) => {
    if (!confirm("Are you sure you want to delete this subscription?")) return;
    try {
      const response = await fetch(`/api/subscriptions/${id}`, {
        method: "DELETE",
        headers: { "Authorization": `Bearer ${localStorage.getItem("token")}` }
      });
      if (response.ok) fetchConfig();
    } catch (err) {
      console.error("Failed to delete subscription:", err);
    }
  };

  const getChannelDisplayName = (id: number) => {
    const cred = credentials.find(c => c.id === id);
    return cred ? cred.name : `ID: ${id}`;
  };

  const getChannelType = (id: number) => {
    const cred = credentials.find(c => c.id === id);
    return cred ? cred.source_type.toUpperCase() : "";
  };

  const getLLMName = (id: number | null) => {
    if (!id) return "None";
    const llm = llmConfigs.find(l => l.id === id);
    return llm ? llm.name : `ID: ${id}`;
  };

  const inputChannels = credentials.filter(c => ['slack', 'telegram', 'lark'].includes(c.source_type));
  const outputChannels = credentials.filter(c => c.source_type === 'dingtalk');

  return (
    <div className="space-y-8 animate-in slide-in-from-bottom-4 duration-500">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">Subscriptions</h2>
          <p className="text-slate-500 mt-1">Manage tasks to bridge input and output channels.</p>
        </div>
        <button 
          onClick={() => setShowAddSub(!showAddSub)}
          className="bg-blue-600 hover:bg-blue-700 text-white px-6 py-3 rounded-2xl shadow-lg shadow-blue-200 flex items-center gap-2 font-bold transition-all active:scale-[0.98]"
        >
          <Plus className="w-5 h-5" />
          New Task
        </button>
      </header>

      {message && (
        <div className={`p-4 rounded-2xl flex items-center gap-3 animate-in fade-in zoom-in duration-300 ${
          message.type === 'success' ? 'bg-emerald-50 border border-emerald-100 text-emerald-700' : 'bg-rose-50 border border-rose-100 text-rose-700'
        }`}>
          {message.type === 'success' ? <CheckCircle2 className="w-5 h-5" /> : <AlertCircle className="w-5 h-5" />}
          <p className="text-sm font-medium">{message.text}</p>
        </div>
      )}

      {showAddSub && (
        <div className="bg-white p-8 rounded-3xl border border-slate-100 shadow-sm animate-in slide-in-from-top-4 duration-300">
          <h3 className="text-xl font-bold text-slate-900 mb-6">Create New Aggregation Task</h3>
          <form onSubmit={handleCreateSub} className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="space-y-2">
                <label className="text-sm font-semibold text-slate-700 ml-1">Input Channel (Source)</label>
                <select 
                  value={selectedSource}
                  onChange={(e) => setSelectedSource(e.target.value)}
                  className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl outline-none focus:ring-4 focus:ring-blue-100 transition-all"
                  required
                >
                  <option value="">Choose an input channel...</option>
                  {inputChannels.map(c => (
                    <option key={c.id} value={c.id}>{c.name} ({c.source_type.toUpperCase()})</option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-semibold text-slate-700 ml-1">Output Channel (Destination)</label>
                <select 
                  value={selectedDest}
                  onChange={(e) => setSelectedDest(e.target.value)}
                  className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl outline-none focus:ring-4 focus:ring-blue-100 transition-all"
                  required
                >
                  <option value="">Choose an output channel...</option>
                  {outputChannels.map(c => (
                    <option key={c.id} value={c.id}>{c.name} ({c.source_type.toUpperCase()})</option>
                  ))}
                </select>
              </div>
            </div>

            <div className="bg-slate-50/50 p-6 rounded-[32px] border border-slate-100 space-y-6">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="bg-white p-2 rounded-xl shadow-sm"><Bot className="w-5 h-5 text-blue-600" /></div>
                  <div>
                    <p className="font-bold text-slate-900">Summarization</p>
                    <p className="text-xs text-slate-500 font-medium">Use AI to condense messages before sending</p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setEnableSummarization(!enableSummarization)}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none ring-2 ring-offset-2 ring-transparent ${enableSummarization ? 'bg-blue-600' : 'bg-slate-200'}`}
                >
                  <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${enableSummarization ? 'translate-x-6' : 'translate-x-1'}`} />
                </button>
              </div>

              {enableSummarization && (
                <div className="space-y-2 animate-in slide-in-from-top-2 duration-200">
                  <label className="text-sm font-semibold text-slate-700 ml-1">Select LLM Configuration</label>
                  <select 
                    value={selectedLLM}
                    onChange={(e) => setSelectedLLM(e.target.value)}
                    className="w-full px-4 py-3 bg-white border border-slate-200 rounded-2xl outline-none focus:ring-4 focus:ring-blue-100 transition-all"
                    required={enableSummarization}
                  >
                    <option value="">Choose an LLM...</option>
                    {llmConfigs.map(l => (
                      <option key={l.id} value={l.id}>{l.name} ({l.provider})</option>
                    ))}
                  </select>
                </div>
              )}
            </div>

            <div className="space-y-2">
              <label className="text-sm font-semibold text-slate-700 ml-1">Sync Interval (seconds)</label>
              <input 
                type="number"
                value={syncInterval}
                onChange={(e) => setSyncInterval(parseInt(e.target.value))}
                min="60"
                className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl outline-none focus:ring-4 focus:ring-blue-100 transition-all"
                required
              />
            </div>

            <div className="flex gap-4 pt-2">
              <button 
                type="submit" 
                disabled={isLoading}
                className="flex-1 bg-slate-900 text-white font-bold py-4 rounded-2xl shadow-lg transition-all active:scale-[0.98] disabled:opacity-70 flex items-center justify-center gap-2"
              >
                {isLoading && <div className="w-5 h-5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                Create Task
              </button>
              <button 
                type="button" 
                onClick={() => setShowAddSub(false)}
                className="px-8 py-4 font-bold text-slate-500 hover:bg-slate-50 rounded-2xl transition-colors"
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4">
        {subscriptions.length === 0 ? (
          <div className="bg-white p-20 rounded-[40px] border border-dashed border-slate-200 text-center">
            <div className="bg-slate-50 w-20 h-20 rounded-3xl flex items-center justify-center mx-auto mb-6">
              <Layers className="w-10 h-10 text-slate-200" />
            </div>
            <h3 className="text-xl font-bold text-slate-900">No active tasks</h3>
            <p className="text-slate-500 mt-2 max-w-sm mx-auto">
              Configure your input and output channels, then create a task to bridge them.
            </p>
          </div>
        ) : (
          subscriptions.map((sub) => (
            <div key={sub.id} className="bg-white p-6 rounded-3xl border border-slate-100 shadow-sm hover:shadow-md transition-all duration-300 flex flex-col md:flex-row md:items-center justify-between gap-6 group">
              <div className="flex items-center gap-6">
                <div className="w-14 h-14 bg-blue-50 text-blue-600 rounded-2xl flex items-center justify-center font-black text-lg shadow-inner ring-1 ring-blue-100/50">
                  #{sub.id}
                </div>
                <div className="flex items-center gap-6">
                  <div>
                    <p className="text-[10px] font-black uppercase tracking-widest text-slate-400 mb-1">Input ({getChannelType(sub.source_credential_id)})</p>
                    <p className="font-bold text-slate-900 leading-tight">{getChannelDisplayName(sub.source_credential_id)}</p>
                  </div>
                  <ArrowRight className="w-5 h-5 text-slate-300" />
                  <div>
                    <p className="text-[10px] font-black uppercase tracking-widest text-slate-400 mb-1">Output ({getChannelType(sub.destination_credential_id)})</p>
                    <p className="font-bold text-slate-900 leading-tight">{getChannelDisplayName(sub.destination_credential_id)}</p>
                  </div>
                </div>
                <div className="h-10 w-[1px] bg-slate-100 mx-2 hidden md:block" />
                <div className="flex items-center gap-2">
                  <div className={`p-2 rounded-lg ${sub.enable_summarization ? 'bg-blue-50 text-blue-600' : 'bg-slate-50 text-slate-400'}`}>
                    <Bot className="w-4 h-4" />
                  </div>
                  <div>
                    <p className="text-[10px] font-black uppercase tracking-widest text-slate-400">Summarization</p>
                    <p className="text-xs font-bold text-slate-700">{sub.enable_summarization ? getLLMName(sub.llm_config_id) : 'Off'}</p>
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-8">
                <div className="text-right">
                  <div className="flex items-center gap-1.5 text-sm text-slate-500 font-bold justify-end mb-1">
                    <Clock className="w-4 h-4 text-slate-400" />
                    {sub.sync_interval}s
                  </div>
                  <p className="text-[10px] text-slate-400 font-medium">
                    Last sync: {sub.last_sync_at ? new Date(sub.last_sync_at).toLocaleTimeString() : 'Never'}
                  </p>
                </div>
                <button 
                  onClick={() => handleDeleteSub(sub.id)}
                  className="p-3 text-slate-300 hover:text-rose-600 hover:bg-rose-50 rounded-2xl transition-all"
                >
                  <Trash2 className="w-5 h-5" />
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default Subscriptions;
