import React, { useState, useEffect } from 'react';
import { Agent } from '../../types/agent';
import { getAgentPersona, getAgentReviews, cloneAgentPersona, retireAgent } from '../../api/agents';
import { getAgentBeads } from '../../api/beads';

interface AgentDetailModalProps {
  agent: Agent;
  isOpen: boolean;
  onClose: () => void;
}

export const AgentDetailModal: React.FC<AgentDetailModalProps> = ({
  agent,
  isOpen,
  onClose,
}) => {
  const [activeTab, setActiveTab] = useState<'identity' | 'performance' | 'activity'>('identity');
  const [personaData, setPersonaData] = useState<any>(null);
  const [reviews, setReviews] = useState<any[]>([]);
  const [beads, setBeads] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<'clone' | 'retire' | null>(null);
  const [showRetireConfirm, setShowRetireConfirm] = useState(false);

  useEffect(() => {
    if (isOpen && agent) {
      loadData();
    }
  }, [isOpen, agent]);

  useEffect(() => {
    if (success) {
      const timer = setTimeout(() => setSuccess(null), 3000);
      return () => clearTimeout(timer);
    }
  }, [success]);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [personaRes, reviewsRes, beadsRes] = await Promise.all([
        getAgentPersona(agent.id),
        getAgentReviews(agent.id),
        getAgentBeads(agent.id),
      ]);
      setPersonaData(personaRes);
      setReviews(reviewsRes);
      setBeads(beadsRes);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agent details');
    } finally {
      setLoading(false);
    }
  };

  const handleClonePersona = async () => {
    setActionLoading('clone');
    setError(null);
    setSuccess(null);
    try {
      await cloneAgentPersona(agent.id);
      setSuccess('Persona cloned successfully');
      setTimeout(() => {
        onClose();
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to clone persona');
    } finally {
      setActionLoading(null);
    }
  };

  const handleRetireAgent = async () => {
    setActionLoading('retire');
    setError(null);
    setSuccess(null);
    try {
      await retireAgent(agent.id);
      setSuccess('Agent retired successfully');
      setShowRetireConfirm(false);
      setTimeout(() => {
        onClose();
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to retire agent');
    } finally {
      setActionLoading(null);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl max-w-2xl w-full max-h-96 overflow-y-auto">
        {/* Header */}
        <div className="border-b px-6 py-4">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-2xl font-bold">{agent.name}</h2>
              <div className="flex gap-2 mt-2">
                <span className="px-3 py-1 bg-blue-100 text-blue-800 rounded-full text-sm font-medium">
                  {agent.role}
                </span>
                <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                  agent.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                }`}>
                  {agent.status}
                </span>
                {agent.performance_grade && (
                  <span className="px-3 py-1 bg-purple-100 text-purple-800 rounded-full text-sm font-medium">
                    Grade: {agent.performance_grade}
                  </span>
                )}
              </div>
            </div>
            <button
              onClick={onClose}
              className="text-gray-500 hover:text-gray-700 text-2xl"
            >
              ×
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="border-b flex">
          {(['identity', 'performance', 'activity'] as const).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`flex-1 py-3 text-center font-medium border-b-2 transition-colors ${
                activeTab === tab
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-gray-600 hover:text-gray-900'
              }`}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </div>

        {/* Content */}
        <div className="p-6">
          {loading && <div className="text-center text-gray-500">Loading...</div>}
          {error && <div className="text-center text-red-500">{error}</div>}
          {success && <div className="text-center text-green-500 font-medium">{success}</div>}

          {!loading && !error && !success && (
            <>
              {activeTab === 'identity' && (
                <div className="space-y-4">
                  {personaData ? (
                    <>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Skill</h3>
                        <div className="bg-gray-50 p-4 rounded text-sm whitespace-pre-wrap">
                          {personaData.skill || 'No skill data'}
                        </div>
                      </div>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Motivation</h3>
                        <div className="bg-gray-50 p-4 rounded text-sm whitespace-pre-wrap">
                          {personaData.motivation || 'No motivation data'}
                        </div>
                      </div>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Personality</h3>
                        <div className="bg-gray-50 p-4 rounded text-sm whitespace-pre-wrap">
                          {personaData.personality || 'No personality data'}
                        </div>
                      </div>
                    </>
                  ) : (
                    <div className="text-gray-500">No persona data available</div>
                  )}
                </div>
              )}

              {activeTab === 'performance' && (
                <div className="space-y-4">
                  {reviews.length > 0 ? (
                    <>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Current Grade</h3>
                        <div className="flex items-center gap-4">
                          <div className="text-4xl font-bold text-blue-600">
                            {agent.performance_grade || 'N/A'}
                          </div>
                          <div className="text-sm text-gray-600">
                            <div>Completion: {reviews[0]?.completion_score || 0}%</div>
                            <div>Efficiency: {reviews[0]?.efficiency_score || 0}%</div>
                            <div>Assist: {reviews[0]?.assist_score || 0}%</div>
                          </div>
                        </div>
                      </div>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Grade History</h3>
                        <div className="flex gap-2">
                          {reviews.slice(0, 5).map((review, idx) => (
                            <div
                              key={idx}
                              className="flex-1 text-center p-2 bg-gray-50 rounded"
                            >
                              <div className="text-sm font-bold">{review.grade}</div>
                              <div className="text-xs text-gray-500">
                                {new Date(review.created_at).toLocaleDateString()}
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    </>
                  ) : (
                    <div className="text-gray-500">No performance data available</div>
                  )}
                </div>
              )}

              {activeTab === 'activity' && (
                <div className="space-y-4">
                  {beads.length > 0 ? (
                    <>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Current Beads</h3>
                        <div className="space-y-2">
                          {beads
                            .filter((b) => b.status === 'open' || b.status === 'working')
                            .map((bead) => (
                              <div key={bead.id} className="p-2 bg-gray-50 rounded text-sm">
                                {bead.title}
                              </div>
                            ))}
                        </div>
                      </div>
                      <div>
                        <h3 className="font-bold text-lg mb-2">Closed This Week</h3>
                        <div className="space-y-2">
                          {beads
                            .filter((b) => b.status === 'closed')
                            .map((bead) => (
                              <div key={bead.id} className="p-2 bg-gray-50 rounded text-sm">
                                {bead.title}
                              </div>
                            ))}
                        </div>
                      </div>
                    </>
                  ) : (
                    <div className="text-gray-500">No activity data available</div>
                  )}
                </div>
              )}
            </>
          )}
        </div>

        {/* Retire Confirmation Dialog */}
        {showRetireConfirm && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
            <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm">
              <h3 className="text-lg font-bold mb-4">Confirm Retire Agent</h3>
              <p className="text-gray-600 mb-6">
                Are you sure you want to retire {agent.name}? This action cannot be undone.
              </p>
              <div className="flex gap-2 justify-end">
                <button
                  onClick={() => setShowRetireConfirm(false)}
                  className="px-4 py-2 text-gray-700 bg-gray-100 rounded hover:bg-gray-200"
                >
                  Cancel
                </button>
                <button
                  onClick={handleRetireAgent}
                  disabled={actionLoading === 'retire'}
                  className="px-4 py-2 text-white bg-red-600 rounded hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {actionLoading === 'retire' ? 'Retiring...' : 'Retire'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Actions */}
        <div className="border-t px-6 py-4 flex gap-2 justify-end">
          <button
            onClick={onClose}
            disabled={actionLoading !== null}
            className="px-4 py-2 text-gray-700 bg-gray-100 rounded hover:bg-gray-200 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Close
          </button>
          <button
            onClick={handleClonePersona}
            disabled={actionLoading !== null}
            className="px-4 py-2 text-white bg-blue-600 rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {actionLoading === 'clone' ? 'Cloning...' : 'Clone Persona'}
          </button>
          <button
            onClick={() => setShowRetireConfirm(true)}
            disabled={actionLoading !== null}
            className="px-4 py-2 text-white bg-red-600 rounded hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Retire Agent
          </button>
        </div>
      </div>
    </div>
  );
};