import { motion } from 'framer-motion';

function App() {
  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-gray-900 text-white p-4">
      <motion.h1 
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8 }}
        className="text-3xl font-bold mb-8"
      >
        Arcana AI Tarot
      </motion.h1>
      
      <motion.div
        whileHover={{ scale: 1.05 }}
        whileTap={{ scale: 0.95 }}
        className="bg-purple-700 px-6 py-3 rounded-lg cursor-pointer shadow-lg"
      >
        Начать расклад (Демо)
      </motion.div>
      
      <p className="mt-8 text-gray-400 text-sm">
        Mini App прототип (Этап 1)
      </p>
    </div>
  );
}

export default App;
