'use client';

import { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { spreads, categoryNames, Spread } from '@/lib/spreads';
import { getCardImageUrl, cardNameToNumber } from '@/lib/cards';

type Step =
  | 'welcome'
  | 'question'
  | 'spread-select'
  | 'payment'
  | 'payment-confirm'
  | 'shuffling'
  | 'cutting'
  | 'revealing';

interface CardData {
  name: string;
  is_reversed: boolean;
}

interface InterpretationData {
  card: CardData;
  interpretation: string;
}

interface ReadingResult {
  intro: string;
  cards: InterpretationData[];
  conclusion: string;
}

const fadeIn = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -20 },
  transition: { duration: 0.5 },
};

export default function HomePage() {
  const [step, setStep] = useState<Step>('welcome');
  const [question, setQuestion] = useState('');
  const [selectedSpread, setSelectedSpread] = useState<Spread | null>(null);
  const [paymentToken, setPaymentToken] = useState('');
  const [revealedCards, setRevealedCards] = useState<InterpretationData[]>([]);
  const [currentCardIndex, setCurrentCardIndex] = useState(0);
  const [result, setResult] = useState<ReadingResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [showConclusion, setShowConclusion] = useState(false);

  const handleStart = () => setStep('question');

  const handleQuestionSubmit = () => {
    if (question.trim().length < 3) {
      setError('Пожалуйста, задайте более конкретный вопрос');
      return;
    }
    setError('');
    setStep('spread-select');
  };

  const handleSpreadSelect = (spread: Spread) => {
    setSelectedSpread(spread);
    setPaymentToken(`TOKEN_${Date.now()}_${Math.random().toString(36).substring(7)}`);
    setStep('payment');
  };

  const handlePaymentConfirm = () => {
    setStep('payment-confirm');
    setTimeout(() => setStep('shuffling'), 2500);
  };

  const handleCutDeck = async () => {
    setStep('cutting');
    setLoading(true);
    setError('');
    setRevealedCards([]);
    setCurrentCardIndex(0);
    setShowConclusion(false);

    try {
      const allCards = Object.keys(cardNameToNumber);
      const shuffled = [...allCards].sort(() => Math.random() - 0.5);
      const selectedCards: CardData[] = shuffled
        .slice(0, selectedSpread!.cardCount)
        .map(name => ({ name, is_reversed: Math.random() < 0.3 }));

      const response = await fetch('/api/read-spread', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          question,
          spread: selectedSpread!.name,
          cards: selectedCards,
        }),
      });

      if (!response.ok) throw new Error(`Ошибка сервера: ${response.status}`);

      const data: ReadingResult = await response.json();
      setResult(data);
      setStep('revealing');
      await revealCardsSequentially(data.cards);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Произошла ошибка';
      setError(`Не удалось получить интерпретацию: ${errorMessage}`);
      setStep('shuffling');
    } finally {
      setLoading(false);
    }
  };

  const revealCardsSequentially = async (cards: InterpretationData[]) => {
    for (let i = 0; i < cards.length; i++) {
      await new Promise(resolve => setTimeout(resolve, 2500));
      setRevealedCards(prev => [...prev, cards[i]]);
      setCurrentCardIndex(i + 1);
    }
    setTimeout(() => setShowConclusion(true), 500);
  };

  const handleNewReading = () => {
    setStep('welcome');
    setQuestion('');
    setSelectedSpread(null);
    setPaymentToken('');
    setRevealedCards([]);
    setCurrentCardIndex(0);
    setResult(null);
    setError('');
    setShowConclusion(false);
  };

  return (
    <div className="min-h-screen py-8 px-4">
      <div className="max-w-5xl mx-auto">
        <motion.header
          className="text-center mb-8"
          initial={{ opacity: 0, y: -30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8 }}
        >
          <h1 className="text-4xl md:text-5xl font-bold mb-2 text-yellow-400">
            ✨ Arcana AI ✨
          </h1>
          <p className="text-lg text-gray-300 font-light">
            Мудрость Таро с искусственным интеллектом
          </p>
        </motion.header>

        <AnimatePresence mode="wait">
          <motion.div
            key={step}
            className="glass p-8 md:p-12"
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            transition={{ duration: 0.4 }}
          >
            {step === 'welcome' && <WelcomeStep onStart={handleStart} />}
            {step === 'question' && (
              <QuestionStep
                question={question}
                setQuestion={setQuestion}
                onSubmit={handleQuestionSubmit}
                error={error}
              />
            )}
            {step === 'spread-select' && (
              <SpreadSelectStep spreads={spreads} onSelect={handleSpreadSelect} />
            )}
            {step === 'payment' && selectedSpread && (
              <PaymentStep spread={selectedSpread} token={paymentToken} onConfirm={handlePaymentConfirm} />
            )}
            {step === 'payment-confirm' && <PaymentConfirmStep />}
            {step === 'shuffling' && (
              <ShufflingStep onCut={handleCutDeck} loading={loading} error={error} />
            )}
            {step === 'cutting' && <CuttingStep />}
            {step === 'revealing' && result && (
              <RevealingStep
                result={result}
                revealedCards={revealedCards}
                currentCardIndex={currentCardIndex}
                spread={selectedSpread!}
                showConclusion={showConclusion}
                onNewReading={handleNewReading}
              />
            )}
          </motion.div>
        </AnimatePresence>

        <footer className="text-center mt-8 text-gray-400 text-sm">
          <p>© 2026 Arcana AI. Все расклады анонимны.</p>
        </footer>
      </div>
    </div>
  );
}

// ===== КОМПОНЕНТЫ ЭТАПОВ =====

function WelcomeStep({ onStart }: { onStart: () => void }) {
  return (
    <motion.div className="text-center" {...fadeIn}>
      {/* ЛОГОТИП АРКАНА */}
      <motion.div
        style={{ backgroundColor: "#292c4c" }} className="relative w-64 h-64 mx-auto mb-8 rounded-2xl shadow-2xl border border-yellow-400/20"
        initial={{ opacity: 0, scale: 0.8 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.8 }}
      >
        <div className="absolute inset-0 rounded-full bg-gradient-to-br from-yellow-400/20 via-purple-400/10 to-blue-400/20 blur-2xl" />
        <img
          src="/arcana-logo.png"
          alt="Arcana AI"
          className="relative w-full h-full object-contain drop-shadow-2xl"
        />
      </motion.div>

      <h2 className="text-3xl md:text-4xl mb-6 text-yellow-400">Добро пожаловать в мир Таро</h2>
      <p className="text-lg text-gray-300 mb-8 leading-relaxed max-w-2xl mx-auto">
        Получите глубокий и персональный расклад Таро с интерпретацией от искусственного интеллекта.
        Каждый расклад уникален и основан на мудрости веков, усиленной современными технологиями.
      </p>
      <button onClick={onStart} className="btn-gold text-lg pulse-gold">
        Начать расклад
      </button>
    </motion.div>
  );
}

function QuestionStep({
  question, setQuestion, onSubmit, error,
}: {
  question: string;
  setQuestion: (q: string) => void;
  onSubmit: () => void;
  error: string;
}) {
  return (
    <motion.div {...fadeIn}>
      <h2 className="text-3xl mb-6 text-center text-yellow-400">Задайте ваш вопрос</h2>
      <p className="text-gray-300 mb-6 text-center">
        Сформулируйте вопрос четко и конкретно.
      </p>
      <textarea
        value={question}
        onChange={(e) => setQuestion(e.target.value)}
        placeholder="Например: Что меня ждет в новом проекте?"
        className="input-mystic min-h-[120px] resize-none mb-4"
        autoFocus
      />
      {error && (
        <motion.p
          className="text-red-400 text-sm mb-4"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          {error}
        </motion.p>
      )}
      <button onClick={onSubmit} className="btn-gold w-full">
        Продолжить
      </button>
    </motion.div>
  );
}

function SpreadSelectStep({ spreads, onSelect }: { spreads: Spread[]; onSelect: (s: Spread) => void }) {
  const grouped = {
    simple: spreads.filter(s => s.category === 'simple'),
    medium: spreads.filter(s => s.category === 'medium'),
    complex: spreads.filter(s => s.category === 'complex'),
  };

  return (
    <motion.div {...fadeIn}>
      <h2 className="text-3xl mb-6 text-center text-yellow-400">Выберите тип расклада</h2>
      <div className="max-h-[600px] overflow-y-auto pr-2 space-y-6">
        {(['simple', 'medium', 'complex'] as const).map(category => (
          <div key={category}>
            <h3 className="text-xl mb-3 text-yellow-400">{categoryNames[category]}</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {grouped[category].map(spread => (
                <motion.button
                  key={spread.id}
                  onClick={() => onSelect(spread)}
                  className="glass p-5 text-left hover:border-yellow-400 transition-all"
                  whileHover={{ scale: 1.03 }}
                  whileTap={{ scale: 0.98 }}
                >
                  <h4 className="text-lg mb-1 text-yellow-300">{spread.name}</h4>
                  <p className="text-gray-400 text-sm mb-3">{spread.description}</p>
                  <div className="flex justify-between items-center">
                    <span className="text-xs text-gray-500">
                      {spread.cardCount} {spread.cardCount === 1 ? 'карта' : 'карт'}
                    </span>
                    <span className="text-yellow-400 font-bold text-lg">{spread.price}₽</span>
                  </div>
                </motion.button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </motion.div>
  );
}

function PaymentStep({ spread, token, onConfirm }: { spread: Spread; token: string; onConfirm: () => void }) {
  return (
    <motion.div className="text-center" {...fadeIn}>
      <h2 className="text-3xl mb-6 text-yellow-400">Оплата расклада</h2>
      <div className="glass p-6 mb-6 text-left">
        <p className="text-lg mb-2"><strong>Расклад:</strong> {spread.name}</p>
        <p className="text-3xl text-yellow-400 font-bold mb-4">Стоимость: {spread.price}₽</p>
        <p className="text-sm text-gray-400 mb-1">
          Номер платежа: <code className="text-xs bg-black/30 px-2 py-1 rounded">{token}</code>
        </p>
        <p className="text-sm text-gray-400">Токен действителен 1 час</p>
      </div>
      <div className="glass p-6 mb-6">
        <p className="text-lg mb-4">Переведите сумму на карту:</p>
        <p className="text-2xl font-mono text-yellow-400 mb-4">2200 0000 0000 0000</p>
        <p className="text-sm text-gray-400">После перевода нажмите кнопку ниже</p>
      </div>
      <button onClick={onConfirm} className="btn-gold text-lg">
        ✅ Я оплатил
      </button>
    </motion.div>
  );
}

function PaymentConfirmStep() {
  return (
    <motion.div className="text-center" {...fadeIn}>
      <motion.div
        className="text-7xl mb-6"
        animate={{ scale: [1, 1.2, 1] }}
        transition={{ duration: 1.5, repeat: Infinity }}
      >
        ⏳
      </motion.div>
      <h2 className="text-3xl mb-4 text-yellow-400">Проверяем оплату...</h2>
      <p className="text-gray-300">Это займет несколько секунд</p>
    </motion.div>
  );
}

function ShufflingStep({ onCut, loading, error }: { onCut: () => void; loading: boolean; error: string }) {
  return (
    <motion.div className="text-center" {...fadeIn}>
      <motion.div
        className="text-7xl mb-6"
        animate={{ rotate: 360 }}
        transition={{ duration: 3, repeat: Infinity, ease: 'linear' }}
      >
        🌪️
      </motion.div>
      <h2 className="text-3xl mb-4 text-yellow-400">Перемешиваем колоду...</h2>
      <p className="text-gray-300 mb-8">Карты заряжаются вашим намерением</p>
      {error && (
        <div className="glass p-4 mb-6 border-red-500">
          <p className="text-red-400">{error}</p>
        </div>
      )}
      <button onClick={onCut} className="btn-gold text-lg" disabled={loading}>
        {loading ? '⏳ Загрузка...' : '✂️ Срезать колоду'}
      </button>
    </motion.div>
  );
}

function CuttingStep() {
  return (
    <motion.div className="text-center" {...fadeIn}>
      <motion.div
        className="text-7xl mb-6"
        animate={{ y: [0, -10, 0] }}
        transition={{ duration: 1.5, repeat: Infinity }}
      >
        🃏
      </motion.div>
      <h2 className="text-3xl mb-4 text-yellow-400">Выкладываем карты...</h2>
      <p className="text-gray-300">Запрашиваем мудрость ИИ...</p>
    </motion.div>
  );
}

function RevealingStep({
  result, revealedCards, currentCardIndex, spread, showConclusion, onNewReading,
}: {
  result: ReadingResult;
  revealedCards: InterpretationData[];
  currentCardIndex: number;
  spread: Spread;
  showConclusion: boolean;
  onNewReading: () => void;
}) {
  return (
    <motion.div {...fadeIn}>
      {/* ВВЕДЕНИЕ С АВАТАРОМ АРКАНЫ */}
      {result.intro && (
        <motion.div
          className="mb-8 p-6 glass"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.8 }}
        >
          <div className="flex items-start gap-4 mb-4">
            <div className="flex-shrink-0">
              <div className="w-16 h-16 rounded-full overflow-hidden border-2 border-yellow-400/40 shadow-lg">
                <img
                  src="/arcana-logo.png"
                  alt="Arcana AI"
                  className="w-full h-full object-cover"
                />
              </div>
            </div>
            <div className="flex-1">
              <h3 className="text-lg text-yellow-400 mb-1">Arcana AI говорит:</h3>
              <p className="text-gray-300 text-sm italic">Интерпретация вашего расклада</p>
            </div>
          </div>
          <p className="text-lg leading-relaxed text-gray-100">{result.intro}</p>
        </motion.div>
      )}

      {/* КАРТЫ */}
      <div className="space-y-6 mb-8">
        <AnimatePresence>
          {revealedCards.map((card, index) => (
            <motion.div
              key={index}
              className="glass p-6"
              initial={{ opacity: 0, x: -50 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.6 }}
            >
              <div className="flex flex-col md:flex-row gap-6">
                <motion.div
                  className="flex-shrink-0"
                  initial={{ rotateY: 180 }}
                  animate={{ rotateY: 0 }}
                  transition={{ duration: 0.8, delay: 0.2 }}
                  style={{ perspective: 1000 }}
                >
                  <img
                    src={getCardImageUrl(card.card.name)}
                    alt={card.card.name}
                    className="w-40 md:w-48 h-auto rounded-lg shadow-2xl border-2 border-yellow-400/30"
                  />
                </motion.div>
                <div className="flex-1">
                  <div className="flex items-center gap-3 mb-3">
                    <div className="w-10 h-10 rounded-full overflow-hidden border border-yellow-400/40">
                      <img
                        src="/arcana-logo.png"
                        alt="Arcana"
                        className="w-full h-full object-cover"
                      />
                    </div>
                    <div>
                      <h3 className="text-xl text-yellow-400">
                        {spread.positions[index] || `Позиция ${index + 1}`}
                      </h3>
                      <p className="text-sm text-gray-400">Arcana AI интерпретирует</p>
                    </div>
                  </div>
                  <p className="text-lg mb-3 font-bold text-gray-100">
                    {card.card.name}
                    {card.card.is_reversed && <span className="text-red-400">  (перевёрнутая)</span>}
                  </p>
                  <p className="text-gray-200 leading-relaxed">{card.interpretation}</p>
                </div>
              </div>
            </motion.div>
          ))}
        </AnimatePresence>
      </div>

      {/* Индикатор загрузки следующих карт */}
      {currentCardIndex < result.cards.length && (
        <motion.div
          className="text-center mb-8"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          <motion.div
            className="text-5xl"
            animate={{ scale: [1, 1.3, 1] }}
            transition={{ duration: 1.5, repeat: Infinity }}
          >
            ✨
          </motion.div>
          <p className="text-gray-300 mt-2">Раскрываем следующую карту... ({currentCardIndex}/{result.cards.length})</p>
        </motion.div>
      )}

      {/* ИТОГ С АВАТАРОМ */}
      {showConclusion && result.conclusion && (
        <motion.div
          className="glass p-8 mb-8 border-2 border-yellow-400/40"
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.3 }}
        >
          <div className="flex items-center gap-4 mb-6">
            <div className="w-16 h-16 rounded-full overflow-hidden border-2 border-yellow-400 shadow-lg">
              <img
                src="/arcana-logo.png"
                alt="Arcana AI"
                className="w-full h-full object-cover"
              />
            </div>
            <div>
              <h3 className="text-2xl text-yellow-400">Итог расклада</h3>
              <p className="text-gray-400 text-sm">Заключение от Arcana AI</p>
            </div>
          </div>
          <p className="text-lg leading-relaxed text-gray-100">{result.conclusion}</p>
        </motion.div>
      )}

      {/* КНОПКА НОВОГО РАСКЛАДА */}
      {showConclusion && (
        <motion.div
          className="text-center"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.6 }}
        >
          <p className="text-xl text-yellow-400 mb-6">
            ✨ Если у вас есть ещё вопросы, начните новый расклад!
          </p>
          <motion.button
            onClick={onNewReading}
            className="btn-gold text-lg pulse-gold"
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
          >
            🔮 Новый расклад
          </motion.button>
        </motion.div>
      )}
    </motion.div>
  );
}
