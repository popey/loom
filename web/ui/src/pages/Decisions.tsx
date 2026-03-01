import React, { useState, useEffect } from 'react';
import { useDecisions } from '../hooks/useDecisions';
import { Decision } from '../types';
import './Decisions.css';

type FilterType = 'all' | 'pending_human' | 'in_progress' | 'recently_closed';

const Decisions: React.FC = () => {
  const { decisions, loading, error, fetchDecisions } = useDecisions();
  const [filter, setFilter] = useState<FilterType>('all');

  useEffect(() => {
    fetchDecisions();
  }, []);

  if (loading) {
    return <div className="decisions-container">Loading decisions...</div>;
  }

  if (error) {
    return <div className="decisions-container error">Error: {error}</div>;
  }

  // Categorize decisions
  const requiresHuman = decisions.filter(
    (d) => d.context?.requires_human === true && d.status !== 'closed'
  );
  const inProgress = decisions.filter(
    (d) => d.context?.requires_human !== true && d.status === 'open'
  );
  const recentlyClosed = decisions.filter((d) => d.status === 'closed').slice(0, 10);

  // Apply filter
  let displayDecisions: Decision[] = [];
  switch (filter) {
    case 'pending_human':
      displayDecisions = requiresHuman;
      break;
    case 'in_progress':
      displayDecisions = inProgress;
      break;
    case 'recently_closed':
      displayDecisions = recentlyClosed;
      break;
    case 'all':
    default:
      displayDecisions = [...requiresHuman, ...inProgress, ...recentlyClosed];
  }

  const hasAnyDecisions = decisions.length > 0;
  const hasHumanEscalations = requiresHuman.length > 0;

  return (
    <div className="decisions-container">
      <div className="decisions-header">
        <h1>Decisions</h1>
        <div className="filter-controls">
          <label htmlFor="decision-filter">Filter:</label>
          <select
            id="decision-filter"
            value={filter}
            onChange={(e) => setFilter(e.target.value as FilterType)}
            className="filter-select"
          >
            <option value="all">All</option>
            <option value="pending_human">Pending Human Review</option>
            <option value="in_progress">In Progress by Agent</option>
            <option value="recently_closed">Recently Closed</option>
          </select>
        </div>
      </div>

      {!hasAnyDecisions && (
        <div className="empty-state">
          <p>No decisions pending human review. Agents are handling decisions autonomously.</p>
        </div>
      )}

      {hasAnyDecisions && (
        <>
          {/* Requires Human Review Section */}
          {(filter === 'all' || filter === 'pending_human') && (
            <section className="decisions-section">
              <h2>Requires Human Review</h2>
              {requiresHuman.length === 0 ? (
                <p className="section-empty">No decisions requiring human review at this time.</p>
              ) : (
                <div className="decisions-grid">
                  {requiresHuman.map((decision) => (
                    <DecisionCard key={decision.id} decision={decision} requiresAction={true} />
                  ))}
                </div>
              )}
            </section>
          )}

          {/* In Progress by Agent Section */}
          {(filter === 'all' || filter === 'in_progress') && (
            <section className="decisions-section">
              <h2>In Progress by Agent</h2>
              {inProgress.length === 0 ? (
                <p className="section-empty">No decisions currently in progress.</p>
              ) : (
                <div className="decisions-grid">
                  {inProgress.map((decision) => (
                    <DecisionCard key={decision.id} decision={decision} requiresAction={false} />
                  ))}
                </div>
              )}
            </section>
          )}

          {/* Recently Decided Section */}
          {(filter === 'all' || filter === 'recently_closed') && (
            <section className="decisions-section">
              <h2>Recently Decided</h2>
              {recentlyClosed.length === 0 ? (
                <p className="section-empty">No recent decisions.</p>
              ) : (
                <div className="decisions-grid">
                  {recentlyClosed.map((decision) => (
                    <DecisionCard key={decision.id} decision={decision} requiresAction={false} />
                  ))}
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
};

interface DecisionCardProps {
  decision: Decision;
  requiresAction: boolean;
}

const DecisionCard: React.FC<DecisionCardProps> = ({ decision, requiresAction }) => {
  const [showDetails, setShowDetails] = useState(false);

  const handleClaim = () => {
    console.log('Claim decision:', decision.id);
  };

  const handleDecide = () => {
    console.log('Decide on decision:', decision.id);
  };

  return (
    <div className={`decision-card ${requiresAction ? 'requires-action' : 'handled'}`}>
      <div className="card-header">
        <h3>{decision.title}</h3>
        <span className={`status-badge status-${decision.status}`}>{decision.status}</span>
      </div>

      <div className="card-body">
        <div className="decision-info">
          <p className="decision-description">{decision.description}</p>

          {decision.assigned_to && (
            <div className="agent-assignment">
              <span className="label">Handled by:</span>
              <span className="agent-name">{decision.assigned_to}</span>
            </div>
          )}

          {decision.context?.type && (
            <div className="decision-type">
              <span className="label">Type:</span>
              <span className="type-value">{decision.context.type}</span>
            </div>
          )}

          {decision.status === 'closed' && decision.context?.rationale && (
            <div className="decision-rationale">
              <span className="label">Rationale:</span>
              <p className="rationale-text">{decision.context.rationale}</p>
            </div>
          )}
        </div>

        {decision.context && Object.keys(decision.context).length > 0 && (
          <button
            className="details-toggle"
            onClick={() => setShowDetails(!showDetails)}
          >
            {showDetails ? 'Hide Details' : 'Show Details'}
          </button>
        )}

        {showDetails && decision.context && (
          <div className="decision-details">
            <pre>{JSON.stringify(decision.context, null, 2)}</pre>
          </div>
        )}
      </div>

      {requiresAction && (
        <div className="card-actions">
          <button className="btn btn-primary" onClick={handleClaim}>
            Claim
          </button>
          <button className="btn btn-success" onClick={handleDecide}>
            Decide
          </button>
        </div>
      )}

      <div className="card-footer">
        <span className="timestamp">
          {new Date(decision.created_at).toLocaleDateString()}
        </span>
      </div>
    </div>
  );
};

export default Decisions;
